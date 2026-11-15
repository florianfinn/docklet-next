// Source: docklet-next/issues/416
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original deterministic overflow, ordering and lifecycle proofs.

package edge

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func blockedDelivery(t *testing.T, tunnel *AgentTunnel, id string) *PendingRequest {
	t.Helper()
	p := &PendingRequest{ResponseCh: make(chan *TunnelMessage, 1), failureCh: make(chan error, 2)}
	p.ResponseCh <- &TunnelMessage{Sequence: -1}
	tunnel.Pending.Store(id, p)
	return p
}

func assertDeliveryJoined(t *testing.T, p *PendingRequest) {
	t.Helper()
	p.deliveryMu.Lock()
	joined := p.drainerDone
	p.deliveryMu.Unlock()
	if joined == nil {
		return
	}
	select {
	case <-joined:
	default:
		t.Error("drainer must be joined")
	}
}

func TestDeliveryInFlightOrdering(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tunnel := &AgentTunnel{}
		p := blockedDelivery(t, tunnel, "order")
		ready, send := make(chan struct{}), make(chan struct{})
		var first atomic.Bool
		p.beforeDelivery = func() {
			if first.CompareAndSwap(false, true) {
				close(ready)
				<-send
			}
		}
		p.deliver(t.Context(), tunnel, &TunnelMessage{ID: "order", Sequence: 0})
		<-ready // The drainer owns the head but has not attempted its send.
		<-p.ResponseCh
		p.deliver(t.Context(), tunnel, &TunnelMessage{ID: "order", Sequence: 1, Type: MessageTypeCommandComplete})
		close(send)
		require.Equal(t, int64(0), (<-p.ResponseCh).Sequence, "in-flight head must not be overtaken")
		last := <-p.ResponseCh
		require.Equal(t, int64(1), last.Sequence)
		require.Equal(t, MessageTypeCommandComplete, last.Type, "terminal last")
		synctest.Wait()
		assertDeliveryJoined(t, p)
		require.Zero(t, tunnel.overflowBytes.Load(), "success reclaims overflow")
		p.Release()
	})
}

func TestDeliveryRequestOverflowBoundary(t *testing.T) {
	for _, extra := range []int{0, 1} {
		name := "exact"
		if extra != 0 {
			name = "plus-one"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				tunnel := &AgentTunnel{}
				p := blockedDelivery(t, tunnel, "cap")
				p.deliver(t.Context(), tunnel, &TunnelMessage{ID: "cap", Body: make([]byte, requestOverflowLimit-2*deliveryMessageOverhead-1)})
				p.deliver(t.Context(), tunnel, &TunnelMessage{ID: "cap", Body: make([]byte, 1+extra)})
				if extra == 0 {
					require.False(t, p.deliveryFailed(), "exact cap remains accepted")
					require.Equal(t, requestOverflowLimit, tunnel.overflowBytes.Load())
					<-p.ResponseCh
					<-p.ResponseCh
					<-p.ResponseCh
					synctest.Wait()
					require.Zero(t, tunnel.overflowBytes.Load(), "success reclaims exact cap")
				} else {
					require.True(t, p.deliveryFailed(), "cap+1 fails the request")
					require.Len(t, p.failureCh, 1, "one overflow failure")
					require.Zero(t, tunnel.overflowBytes.Load(), "failure reclaims overflow")
				}
				p.Release()
				synctest.Wait()
				assertDeliveryJoined(t, p)
				other, err := registerPendingRequestInternal(tunnel, "other")
				require.NoError(t, err)
				other.deliver(t.Context(), tunnel, &TunnelMessage{ID: "other", Type: MessageTypeCommandComplete})
				require.Equal(t, MessageTypeCommandComplete, (<-other.ResponseCh).Type, "other requests complete")
				other.Release()
			})
		})
	}
}

func TestDeliveryTunnelOverflowBoundary(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tunnel := &AgentTunnel{}
		body := make([]byte, requestOverflowLimit-deliveryMessageOverhead)
		var requests []*PendingRequest
		for _, id := range []string{"a", "b", "c", "d"} {
			p := blockedDelivery(t, tunnel, id)
			requests = append(requests, p)
			p.deliver(t.Context(), tunnel, &TunnelMessage{ID: id, Body: body})
			require.False(t, p.deliveryFailed())
		}
		require.Equal(t, tunnelOverflowLimit, tunnel.overflowBytes.Load(), "exact aggregate cap accepted")
		fifth := blockedDelivery(t, tunnel, "e")
		fifth.deliver(t.Context(), tunnel, &TunnelMessage{ID: "e"})
		require.True(t, fifth.deliveryFailed(), "aggregate cap refuses overflowing request")
		require.Len(t, fifth.failureCh, 1)
		for _, p := range requests {
			p.Release()
		}
		synctest.Wait()
		require.Zero(t, tunnel.overflowBytes.Load(), "abort reclaims aggregate overflow")
		for _, p := range requests {
			assertDeliveryJoined(t, p)
		}
		fresh := blockedDelivery(t, tunnel, "reuse")
		fresh.deliver(t.Context(), tunnel, &TunnelMessage{ID: "reuse", Body: body})
		require.False(t, fresh.deliveryFailed(), "aggregate capacity can be reused")
		fresh.Release()
		synctest.Wait()
		assertDeliveryJoined(t, fresh)
	})
}

func TestDeliveryEmptyFramesAreCharged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tunnel := &AgentTunnel{}
		p := blockedDelivery(t, tunnel, "empty")
		msg := &TunnelMessage{ID: "empty", Headers: map[string]string{"Content-Type": "text/plain"}}
		for range requestOverflowLimit / 512 {
			p.deliver(t.Context(), tunnel, msg)
		}
		require.Equal(t, requestOverflowLimit, tunnel.overflowBytes.Load(), "zero-body frames have minimum item cost")
		require.False(t, p.deliveryFailed())
		p.deliver(t.Context(), tunnel, msg)
		require.True(t, p.deliveryFailed(), "empty-frame overflow fails")
		require.Zero(t, tunnel.overflowBytes.Load())
		synctest.Wait()
		assertDeliveryJoined(t, p)
	})
}

func TestDeliveryEndArbitration(t *testing.T) {
	for _, outcome := range []string{"release", "completion", "failure-close"} {
		t.Run(outcome, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				tunnel := &AgentTunnel{done: make(chan struct{})}
				p := blockedDelivery(t, tunnel, "end")
				var failures atomic.Int32
				p.onFailure = func() { failures.Add(1) }
				p.deliver(t.Context(), tunnel, &TunnelMessage{ID: "end", Type: MessageTypeCommandComplete, Status: 200})
				synctest.Wait()
				if outcome == "completion" {
					<-p.ResponseCh
					terminal := <-p.ResponseCh
					require.Equal(t, MessageTypeCommandComplete, terminal.Type, "completed terminal retained")
					p.Release()
				} else if outcome == "release" {
					p.Release()
				}
				close(tunnel.done)
				time.Sleep(streamDeliveryTimeout)
				synctest.Wait()
				// A second synchronized failure competes with the end already chosen.
				p.deliveryMu.Lock()
				p.failLocked(context.Background(), tunnel, "end", "delivery timeout")
				p.deliveryMu.Unlock()
				synctest.Wait()
				want := int32(0)
				if outcome == "failure-close" {
					want = 1
				}
				require.Equal(t, want, failures.Load(), "shared end arbitration permits exactly one outcome")
				require.Len(t, p.failureCh, int(want), "failure is signalled once without blocking")
				require.Zero(t, tunnel.overflowBytes.Load())
				assertDeliveryJoined(t, p)
			})
		})
	}
}

func TestDeliveryConcurrentEnd(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for range 32 {
			tunnel := &AgentTunnel{}
			p := blockedDelivery(t, tunnel, "race")
			var failures atomic.Int32
			p.onFailure = func() { failures.Add(1) }
			p.deliver(t.Context(), tunnel, &TunnelMessage{ID: "race"})
			synctest.Wait()
			start := make(chan struct{})
			joined := make(chan struct{}, 2)
			go func() { <-start; p.Release(); joined <- struct{}{} }()
			go func() {
				<-start
				p.deliveryMu.Lock()
				p.failLocked(t.Context(), tunnel, "race", "delivery timeout")
				p.deliveryMu.Unlock()
				joined <- struct{}{}
			}()
			close(start)
			<-joined
			<-joined
			synctest.Wait()
			want := int32(0)
			if p.deliveryFailed() {
				want = 1
			}
			require.Equal(t, want, failures.Load(), "concurrent end has one winning outcome")
			require.Len(t, p.failureCh, int(want))
			require.Zero(t, tunnel.overflowBytes.Load())
			assertDeliveryJoined(t, p)
		}
	})
}

func TestDeliveryFailureSignalNeverBlocks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tunnel := &AgentTunnel{}
		p := blockedDelivery(t, tunnel, "full-failure")
		p.failureCh = make(chan error, 1)
		p.failureCh <- context.Canceled
		p.deliver(t.Context(), tunnel, &TunnelMessage{ID: "full-failure"})
		synctest.Wait()
		time.Sleep(streamDeliveryTimeout)
		synctest.Wait()
		assertDeliveryJoined(t, p)
		require.True(t, p.deliveryFailed(), "failure ends even when signal channel was full")
		require.Zero(t, tunnel.overflowBytes.Load())
	})
}
