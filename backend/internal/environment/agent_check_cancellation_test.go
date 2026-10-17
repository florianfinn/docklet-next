// Source: docklet-next/issues/81
// Revision: 79669bb43fa8ea72ea38bd55866b9b78b8a72a93
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original shared-check caller cancellation regression tests.

package environment

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

func TestAgentEdgeWaiterSurvivesLeaderCancellation(t *testing.T) {
	for _, forced := range []bool{false, true} {
		name := "ordinary"
		if forced {
			name = "forced"
		}
		t.Run(name, func(t *testing.T) {
			f := newAgentFixture(t, "edge", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			edge.GetRegistry().Unregister(f.id)
			f.conn = &agentMemoryConn{send: f.conn.send}
			f.tunnel = edge.NewAgentTunnelWithConn(f.id, f.conn)
			f.tunnel.APIKeyID, f.tunnel.TokenHash = "example", edge.AgentTokenHash(f.token)
			leaderCtx, cancelLeader := context.WithCancel(ctx)
			defer cancelLeader()
			var workers sync.WaitGroup
			defer func() { cancel(); workers.Wait() }()
			leader := make(chan error, 1)
			workers.Add(1)
			go func() {
				defer workers.Done()
				_, err := f.service.ExecuteRemoteRequest(leaderCtx, f.id, http.MethodGet, "/api/environments/0/containers", nil)
				leader <- err
			}()
			require.Eventually(t, func() bool {
				return edge.GetDemandRegistry().DesiredStatus(f.id, false, time.Now()) == edge.TunnelStatusRequired
			}, time.Second, time.Millisecond)
			waiter := make(chan error, 1)
			workers.Add(1)
			go func() {
				defer workers.Done()
				if forced {
					_, err := f.service.CheckAgentCompatibility(ctx, f.id)
					waiter <- err
					return
				}
				_, err := f.service.ExecuteRemoteRequest(ctx, f.id, http.MethodGet, "/api/environments/0/containers", nil)
				waiter <- err
			}()
			require.Eventually(t, func() bool { return agentCheckWaiters() == 2 }, time.Second, time.Millisecond)
			cancelLeader()
			select {
			case err := <-leader:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(time.Second):
				t.Fatal("the leader must leave promptly without ending the shared attempt")
			}
			select {
			case err := <-waiter:
				t.Fatalf("live waiter inherited the leader's cancellation before tunnel arrival: %v", err)
			case <-time.After(300 * time.Millisecond):
			}
			edge.GetRegistry().Register(f.id, f.tunnel)
			select {
			case err := <-waiter:
				require.NoError(t, err, "a live waiter must use the tunnel arriving after leader cancellation")
			case <-ctx.Done():
				t.Fatal("live waiter did not recover when the tunnel arrived")
			}
			require.False(t, f.conn.IsClosed())
			wantReads := 1
			if forced {
				wantReads = 2
			}
			require.Equal(t, wantReads, f.count("GET /api/app-version"))
		})
	}
}

func TestAgentConnectionWaiterSurvivesLeaderCancellation(t *testing.T) {
	f := newAgentFixture(t, "direct", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, f.service.db.Model(&Environment{}).Where("id = ?", f.id).Update("status", "online").Error)
	started, release := make(chan struct{}), make(chan struct{})
	transport := f.service.httpClient.Transport
	f.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/app-version" {
			select {
			case <-started:
			default:
				close(started)
			}
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		}
		return transport.RoundTrip(r)
	})
	leaderCtx, cancelLeader := context.WithCancel(ctx)
	defer cancelLeader()
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	leader := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		_, err := f.service.ExecuteRemoteRequest(leaderCtx, f.id, http.MethodGet, "/api/environments/0/containers", nil)
		leader <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("leader did not start its version read")
	}
	type result struct {
		status string
		err    error
	}
	waiter := make(chan result, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		status, err := f.service.TestConnection(ctx, f.id, nil)
		waiter <- result{status: status, err: err}
	}()
	require.Eventually(t, func() bool { return agentCheckWaiters() == 2 }, time.Second, time.Millisecond)
	cancelLeader()
	select {
	case err := <-leader:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("the leader must leave promptly without ending the version read")
	}
	// Keep the version response pending while the cancelled caller leaves.
	select {
	case outcome := <-waiter:
		t.Fatalf("live connection check inherited cancellation: status=%s err=%v", outcome.status, outcome.err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	select {
	case outcome := <-waiter:
		require.NoError(t, outcome.err)
		require.Equal(t, "online", outcome.status)
	case <-ctx.Done():
		t.Fatal("connection check did not complete")
	}
	env, err := f.service.GetEnvironmentByID(ctx, f.id)
	require.NoError(t, err)
	require.Equal(t, "online", env.Status, "a live connection check must keep the reachable host online")
	require.Equal(t, 2, f.count("GET /api/app-version"), "the forced health refresh still performs its own version read")
}
