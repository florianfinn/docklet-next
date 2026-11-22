// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original IPN bus watcher with reconnect, reseed and bounded status refresh.

package tailnet

import (
	"context"
	"log/slog"
	"tailscale.com/health"
	"tailscale.com/tsconst"
	"time"

	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"

	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

// watch keeps the observed status current until ctx ends: the bus seeds on every
// (re)subscribe, and any change triggers a Status refresh at most once per interval.
func (m *Manager) watch(ctx context.Context, gen int64, node Node) {
	kick := make(chan struct{}, 1)
	go m.refreshLoop(ctx, gen, node, kick)
	trigger := func() {
		select {
		case kick <- struct{}{}:
		default:
		}
	}
	trigger()
	backoff := time.Second
	for ctx.Err() == nil {
		watcher, err := node.Watch(ctx)
		if err != nil {
			slog.Debug("Tailnet bus subscribe failed", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, m.cfg.RefreshInterval)
			continue
		}
		backoff = time.Second
		m.consume(ctx, gen, watcher, trigger)
		_ = watcher.Close()
		trigger()
	}
}

func (m *Manager) consume(ctx context.Context, gen int64, watcher Watcher, trigger func()) {
	stop := context.AfterFunc(ctx, func() { _ = watcher.Close() })
	defer stop()
	for {
		n, err := watcher.Next()
		if err != nil {
			return
		}
		if n.Health != nil {
			m.applyHealth(gen, n.Health)
		}
		if n.InitialStatus != nil {
			m.applyStatus(gen, n.InitialStatus)
		}
		if relevant(n) {
			trigger()
		}
	}
}

func relevant(n ipn.Notify) bool {
	return n.State != nil || n.Health != nil || n.SelfChange != nil || len(n.PeersChanged) > 0 ||
		len(n.PeersRemoved) > 0 || len(n.PeerChangedPatch) > 0 || n.ErrMessage != nil || n.LoginFinished != nil
}

// refreshLoop runs one Status call per kick, waits the interval before the next,
// and also refreshes periodically so control-grace expiry is observed without bus traffic.
func (m *Manager) refreshLoop(ctx context.Context, gen int64, node Node, kick <-chan struct{}) {
	periodic := 3 * m.cfg.RefreshInterval
	for {
		select {
		case <-ctx.Done():
			return
		case <-kick:
		case <-time.After(periodic):
		}
		statusCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		status, err := node.Status(statusCtx)
		cancel()
		if err == nil {
			m.applyStatus(gen, status)
		} else if ctx.Err() == nil {
			slog.Debug("Tailnet status refresh failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(m.cfg.RefreshInterval):
		}
	}
}

// coordinationWarnings are the health warnables meaning the control server no longer delivers network maps;
// other warnings do not change the connection state.
var coordinationWarnings = []health.WarnableCode{tsconst.HealthWarnableNotInMapPoll, tsconst.HealthWarnableMapResponseTimeout}

// applyHealth records whether the bus health state of generation gen reports coordination trouble.
func (m *Manager) applyHealth(gen int64, state *health.State) {
	warning := false
	for _, code := range coordinationWarnings {
		if _, ok := state.Warnings[code]; ok {
			warning = true
		}
	}
	m.mu.Lock()
	if m.gen != gen {
		m.mu.Unlock()
		return
	}
	m.coordWarning = warning
	if m.status != nil {
		m.deriveConnectionLocked()
	}
	fns := m.notifyLocked()
	m.mu.Unlock()
	fire(fns)
}

// applyStatus records a status snapshot of generation gen and derives the connection state.
func (m *Manager) applyStatus(gen int64, status *ipnstate.Status) {
	if status == nil {
		return
	}
	m.mu.Lock()
	if m.gen != gen {
		m.mu.Unlock()
		return
	}
	m.status = status
	m.deriveConnectionLocked()
	fns := m.notifyLocked()
	m.mu.Unlock()
	fire(fns)
}

// deriveConnectionLocked: Running is control-unreachable while the bus reports coordination trouble or Self
// has been offline longer than the grace period.
func (m *Manager) deriveConnectionLocked() {
	status := m.status
	now := m.cfg.Now()
	switch status.BackendState {
	case ipn.Running.String():
		if status.Self != nil && !status.Self.Online {
			if m.offlineSince.IsZero() {
				m.offlineSince = now
			}
		} else {
			m.offlineSince = time.Time{}
		}
		if m.coordWarning || !m.offlineSince.IsZero() && now.Sub(m.offlineSince) > m.cfg.ControlGrace {
			m.conn = tailnettypes.Connection{State: "control-unreachable", Code: tailnettypes.CodeCoordinationUnreachable}
		} else {
			m.conn = tailnettypes.Connection{State: "connected"}
		}
	case ipn.NeedsLogin.String(), ipn.NeedsMachineAuth.String():
		m.conn = tailnettypes.Connection{State: "error", Code: tailnettypes.CodeNotConnected}
	default:
		m.conn = tailnettypes.Connection{State: "joining"}
	}
}
