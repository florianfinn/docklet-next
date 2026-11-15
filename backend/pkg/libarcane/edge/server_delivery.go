// Source: docklet-next/issues/416
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original ordered per-request overflow delivery and lifecycle arbitration.

package edge

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const (
	requestOverflowLimit    int64 = 16 << 20
	tunnelOverflowLimit     int64 = 64 << 20
	deliveryMessageOverhead int64 = 512
)

func deliveryCharge(msg *TunnelMessage) int64 { return int64(len(msg.Body)) + deliveryMessageOverhead }

// initDeliveryLocked also supports pending requests constructed by stream consumers.
func (p *PendingRequest) initDeliveryLocked() {
	if p.done == nil {
		p.done = make(chan struct{})
	}
	if p.flushCh == nil {
		p.flushCh = make(chan struct{})
		close(p.flushCh)
	}
}

func (s *TunnelServer) deliverPending(ctx context.Context, tunnel *AgentTunnel, msg *TunnelMessage) {
	entry, ok := tunnel.Pending.Load(msg.ID)
	if !ok {
		return
	}
	p, ok := entry.(*PendingRequest)
	if !ok {
		return
	}
	p.deliver(ctx, tunnel, msg)
}

func (p *PendingRequest) deliver(ctx context.Context, tunnel *AgentTunnel, msg *TunnelMessage) {
	p.deliveryMu.Lock()
	defer p.deliveryMu.Unlock()
	p.initDeliveryLocked()
	if p.released || p.failed {
		return
	}
	p.deliveryTunnel = tunnel
	// An empty deque is insufficient: the drainer still owns the sending head.
	if !p.draining && len(p.queue) == 0 {
		select {
		case p.ResponseCh <- msg:
			return
		default:
		}
	}
	charge := deliveryCharge(msg)
	if p.charged+charge > requestOverflowLimit {
		p.failLocked(ctx, tunnel, msg.ID, "overflow")
		return
	}
	// A CAS reserves aggregate bytes without ever making the bound available twice.
	for {
		total := tunnel.overflowBytes.Load()
		if total+charge > tunnelOverflowLimit {
			p.failLocked(ctx, tunnel, msg.ID, "overflow")
			return
		}
		if tunnel.overflowBytes.CompareAndSwap(total, total+charge) {
			break
		}
	}
	p.charged += charge
	p.queue = append(p.queue, msg)
	if !p.draining {
		p.draining = true
		p.flushCh = make(chan struct{})
		p.drainerDone = make(chan struct{})
		go p.drain(context.WithoutCancel(ctx), tunnel, msg.ID, p.drainerDone)
	}
}

// drain deliberately ignores tunnel.done: received terminal evidence must survive
// transport close. Release or failure ends ownership of the remaining overflow.
func (p *PendingRequest) drain(ctx context.Context, tunnel *AgentTunnel, id string, joined chan struct{}) {
	defer close(joined)
	timer := time.NewTimer(streamDeliveryTimeout)
	defer timer.Stop()
	for {
		p.deliveryMu.Lock()
		if p.released || p.failed {
			p.deliveryMu.Unlock()
			return
		}
		if len(p.queue) == 0 {
			p.draining = false
			close(p.flushCh)
			p.deliveryMu.Unlock()
			return
		}
		head := p.queue[0]
		done := p.done
		before := p.beforeDelivery
		p.deliveryMu.Unlock()
		if before != nil {
			before()
		}
		timer.Reset(streamDeliveryTimeout)
		select {
		case <-done:
			return
		case <-timer.C:
			p.deliveryMu.Lock()
			p.failLocked(ctx, tunnel, id, "delivery timeout")
			p.deliveryMu.Unlock()
			return
		case p.ResponseCh <- head:
			timer.Stop()
			p.deliveryMu.Lock()
			if p.released || p.failed {
				p.deliveryMu.Unlock()
				return
			}
			charge := deliveryCharge(head)
			p.queue[0] = nil
			p.queue = p.queue[1:]
			p.charged -= charge
			tunnel.overflowBytes.Add(-charge)
			p.deliveryMu.Unlock()
		}
	}
}

func (p *PendingRequest) clearOverflowLocked() {
	if p.deliveryTunnel != nil {
		p.deliveryTunnel.overflowBytes.Add(-p.charged)
	}
	p.charged = 0
	p.queue = nil
	if p.draining {
		p.draining = false
		close(p.flushCh)
	}
}

func (p *PendingRequest) failLocked(ctx context.Context, tunnel *AgentTunnel, id, cause string) {
	p.endOnce.Do(func() {
		p.failed = true
		close(p.done)
		p.clearOverflowLocked()
		tunnel.Pending.CompareAndDelete(id, p)
		err := fmt.Errorf("%s for pending request %s", cause, id)
		slog.WarnContext(ctx, "Edge pending delivery failed", "requestId", id, "cause", cause)
		select {
		case p.failureCh <- err:
		default:
		}
		if p.onFailure != nil {
			go p.onFailure()
		}
	})
}

// Release ends consumer ownership before its pending entry is removed. It wins
// against a later delivery timeout and returns every remaining overflow charge.
func (p *PendingRequest) Release() {
	p.deliveryMu.Lock()
	defer p.deliveryMu.Unlock()
	p.initDeliveryLocked()
	p.endOnce.Do(func() {
		p.released = true
		close(p.done)
		p.clearOverflowLocked()
	})
}

func (p *PendingRequest) deliveryFailed() bool {
	p.deliveryMu.Lock()
	defer p.deliveryMu.Unlock()
	return p.failed
}

func (p *PendingRequest) flushed() <-chan struct{} {
	p.deliveryMu.Lock()
	defer p.deliveryMu.Unlock()
	p.initDeliveryLocked()
	return p.flushCh
}

func (p *PendingRequest) closeAgent(tunnel *AgentTunnel, id string, kind TunnelMessageType) {
	p.cleanupOnce.Do(func() { _ = tunnel.Conn.Send(&TunnelMessage{ID: id, Type: kind}) })
}
