// Source: docklet-next/issues/599
// Revision: 22319d6f16ef401b1c247c7a4eae599676564095
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original revision-fenced setup-test retry of a rejected hub join.

package tailnet

import (
	"context"
	dto "github.com/florianfinn/docklet-next/types/tailnet"
)

// RetryJoin retries a rejected enrollment without replacing its credential revision.
// False means another lifecycle superseded this test; timeout/cancellation leaves the observed failure intact.
func (m *Manager) RetryJoin(ctx context.Context, expectedCredRevision string) bool {
	m.mu.Lock()
	if m.credentialRevisionLocked() != expectedCredRevision {
		m.mu.Unlock()
		return false
	}
	if !m.enabled || m.conn.State != "error" || (m.conn.Code != dto.CodeCredentialRejected && m.conn.Code != dto.CodeTagsNotPermitted) {
		m.mu.Unlock()
		return true
	}
	changed := make(chan struct{}, 1)
	id := m.nextListener
	m.nextListener++
	m.listeners[id] = func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}
	gen, old, fns := m.restartLocked(m.enabled, m.cred)
	cred := m.cred
	m.mu.Unlock()
	defer func() { m.mu.Lock(); delete(m.listeners, id); m.mu.Unlock() }()
	m.startGeneration(ctx, gen, old, fns, true, cred)
	waitCtx, cancel := context.WithTimeout(ctx, m.cfg.JoinTimeout)
	defer cancel()
	for {
		m.mu.Lock()
		current := m.gen == gen && m.credentialRevisionLocked() == expectedCredRevision
		terminal := m.conn.State == "connected" || m.conn.State == "error" || m.conn.State == "control-unreachable"
		m.mu.Unlock()
		if !current {
			return false
		}
		if terminal {
			return true
		}
		select {
		case <-changed:
		case <-waitCtx.Done():
			m.mu.Lock()
			current = m.gen == gen && m.credentialRevisionLocked() == expectedCredRevision
			m.mu.Unlock()
			return current
		}
	}
}
