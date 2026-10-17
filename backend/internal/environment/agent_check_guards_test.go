// Source: docklet-next/issues/81
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original compatibility guard regression tests.

package environment

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAgentForcedCheckStopsOnSessionRevocation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	f := newAgentFixture(t, "edge", version.Info{})
	edge.GetRegistry().Unregister(f.id)
	done := make(chan error, 1)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		_, err := f.service.CheckAgentCompatibility(ctx, f.id)
		done <- err
	}()
	defer func() { cancel(); <-joined }()
	require.Eventually(t, func() bool {
		return edge.GetDemandRegistry().DesiredStatus(f.id, false, time.Now()) == edge.TunnelStatusRequired
	}, time.Second, time.Millisecond)
	f.service.agentRuntime.mu.Lock()
	session := f.service.agentRuntime.sessions[f.id]
	f.service.agentRuntime.mu.Unlock()
	session.checkMu.Lock()
	check := session.check
	session.checkMu.Unlock()
	require.NotNil(t, check)
	// Revoke during acquisition, before nested version access can supply cancellation.
	f.service.revokeAgentSessions(f.id)
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("forced caller must stop when its credential session is revoked")
	}
	select {
	case <-check.done:
		require.ErrorIs(t, check.err, remenv.ErrEnvironmentUnavailable)
	case <-time.After(time.Second):
		t.Fatal("forced check must stop acquiring a tunnel when its credential session is revoked")
	}
	require.NoError(t, ctx.Err(), "the caller remains live after session revocation")
	require.Empty(t, f.calls)
}

func TestAgentCheckRevalidatesEnabledBeforeCachedVerdict(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	f := newAgentFixture(t, "direct", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
	_, err := f.service.CheckAgentCompatibility(ctx, f.id)
	require.NoError(t, err)
	observed, release := make(chan struct{}), make(chan struct{})
	var blocked atomic.Bool
	require.NoError(t, f.service.db.Callback().Query().After("gorm:query").Register("test:disable-before-verdict", func(db *gorm.DB) {
		if db.Statement.Context.Value(staleAgentObserver{}) == true && blocked.CompareAndSwap(false, true) {
			close(observed)
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
	}))
	done := make(chan error, 1)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		_, cleanup, err := f.service.AgentAccess(context.WithValue(ctx, staleAgentObserver{}, true), f.id, &f.token, http.MethodGet, "/api/environments/0/containers")
		if cleanup != nil {
			cleanup()
		}
		done <- err
	}()
	defer func() { cancel(); <-joined }()
	select {
	case <-observed:
	case <-ctx.Done():
		t.Fatal("request did not read the enabled snapshot")
	}
	// Isolate the interval between the disable write and session revocation.
	require.NoError(t, f.service.db.Model(&Environment{}).Where("id = ?", f.id).Update("enabled", false).Error)
	close(release)
	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrInvalidEnvironmentAccessToken, "a cached verdict cannot authorise a disabled host")
	case <-ctx.Done():
		t.Fatal("request remained blocked after disable")
	}
	require.Equal(t, 1, f.count("GET /api/app-version"))
	require.Zero(t, f.count("GET /api/environments/0/containers"))
}

func TestAgentVersionReadStaysPinnedToAcquiredTunnel(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	f := newAgentFixture(t, "edge", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
	_, cleanup, err := f.service.AgentAccess(ctx, f.id, &f.token, http.MethodGet, "/api/health")
	require.NoError(t, err)
	defer cleanup()
	acquired := f.tunnel
	var queries atomic.Int32
	var swapped atomic.Bool
	require.NoError(t, f.service.db.Callback().Query().After("gorm:query").Register("test:replace-before-version-read", func(*gorm.DB) {
		// The third lookup resolves the version target, after the check acquired T1.
		if queries.Add(1) == 3 {
			f.conn = &agentMemoryConn{send: f.conn.send}
			f.tunnel = edge.NewAgentTunnelWithConn(f.id, f.conn)
			f.tunnel.APIKeyID, f.tunnel.TokenHash = "example", edge.AgentTokenHash(f.token)
			edge.GetRegistry().Register(f.id, f.tunnel)
			swapped.Store(true)
		}
	}))
	info, err := f.service.CheckAgentCompatibility(ctx, f.id)
	require.True(t, swapped.Load(), "replace the tunnel after acquisition, before version forwarding")
	require.NotSame(t, acquired, f.tunnel)
	require.Error(t, err, "a version read bound to T1 must reject replacement T2")
	require.Nil(t, info)
	require.Zero(t, f.count("GET /api/app-version"), "the acquired verdict must never describe another tunnel's version")
	require.False(t, f.conn.IsClosed(), "the rejected old check must leave the current tunnel open")
	env, readErr := f.service.GetEnvironmentByID(ctx, f.id)
	require.NoError(t, readErr)
	require.Equal(t, "pending", env.Status)
	require.Nil(t, env.LastSeen)
}
