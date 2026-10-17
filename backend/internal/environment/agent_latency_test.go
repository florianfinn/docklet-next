// Source: docklet-next/issues/81
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original compatibility concurrency regression tests.

package environment

import (
	"context"
	"net/http"
	"runtime"
	"strings"
	"sync"
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

func agentCheckWaiters() int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	count := 0
	for _, stack := range strings.Split(string(buf[:n]), "\n\n") {
		if strings.Contains(stack, "waitAgentCheck(") {
			count++
		}
	}
	return count
}

func TestAgentConcurrentUnavailableChecksShareOutcome(t *testing.T) {
	for _, mode := range []string{"edge", "direct"} {
		t.Run(mode, func(t *testing.T) {
			const cap = 400 * time.Millisecond
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			f := newAgentFixture(t, mode, version.Info{})
			var reads atomic.Int32
			if mode == "edge" {
				edge.GetRegistry().Unregister(f.id)
			} else {
				f.service.httpClient.Timeout = cap
				f.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
					reads.Add(1)
					<-r.Context().Done()
					return nil, r.Context().Err()
				})
			}
			var workers sync.WaitGroup
			defer func() { cancel(); workers.Wait() }()
			done := make(chan error, 3)
			request := func(requestCtx context.Context) {
				defer workers.Done()
				_, err := f.service.ExecuteRemoteRequest(requestCtx, f.id, http.MethodGet, "/api/environments/0/containers", nil)
				done <- err
			}
			// Bound only the leader's Edge acquisition; followers keep independent live contexts.
			leaderCtx := ctx
			if mode == "edge" {
				var finish context.CancelFunc
				leaderCtx, finish = context.WithTimeout(ctx, cap)
				defer finish()
			}
			started := time.Now()
			workers.Add(1)
			go request(leaderCtx)
			require.Eventually(t, func() bool {
				if mode == "direct" {
					return reads.Load() == 1
				}
				return edge.GetDemandRegistry().DesiredStatus(f.id, false, time.Now()) == edge.TunnelStatusRequired
			}, cap/2, time.Millisecond)
			workers.Add(2)
			go request(ctx)
			go request(ctx)
			require.Eventually(t, func() bool { return agentCheckWaiters() == 2 }, cap/2, time.Millisecond)
			for range 3 {
				select {
				case err := <-done:
					require.Error(t, err)
					if mode == "edge" {
						require.ErrorIs(t, err, remenv.ErrEnvironmentUnavailable)
					} else {
						require.ErrorIs(t, err, context.DeadlineExceeded)
					}
				case <-time.After(2 * cap):
					t.Fatal("concurrent failures must share one bounded attempt")
				}
			}
			elapsed := time.Since(started)
			require.Less(t, elapsed, 2*cap, "three failures must finish within about one attempt")
			t.Logf("3 %s requests completed in %s (attempt cap %s)", mode, elapsed, cap)
			if mode == "direct" {
				require.EqualValues(t, 1, reads.Load(), "overlapping version failures must be shared")
				_, err := f.service.ExecuteRemoteRequest(ctx, f.id, http.MethodGet, "/api/environments/0/containers", nil)
				require.Error(t, err)
				require.EqualValues(t, 2, reads.Load(), "a later request must retry a failed check")
			}
			require.Zero(t, f.count("GET /api/environments/0/containers"))
		})
	}
}

func TestAgentCompatibilityWaitObservesCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	f := newAgentFixture(t, "direct", version.Info{})
	started := make(chan struct{})
	f.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	leader := make(chan error, 1)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	workers.Add(1)
	go func() {
		defer workers.Done()
		_, err := f.service.CheckAgentCompatibility(ctx, f.id)
		leader <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("version read did not start")
	}
	waitCtx, finish := context.WithCancel(ctx)
	defer finish()
	waiter := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		_, err := f.service.CheckAgentCompatibility(waitCtx, f.id)
		waiter <- err
	}()
	require.Eventually(t, func() bool { return agentCheckWaiters() == 1 }, time.Second, time.Millisecond)
	finish()
	select {
	case err := <-waiter:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("a cancelled caller must leave the in-flight check promptly")
	}
	require.NoError(t, ctx.Err(), "cancelling a waiter must leave the checking caller alive")
	select {
	case <-leader:
		t.Fatal("cancelling a waiter must not end the shared attempt")
	default:
	}
}

func TestAgentForcedCheckDoesNotReuseConcurrentCachedVerdict(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	f := newAgentFixture(t, "direct", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
	_, err := f.service.CheckAgentCompatibility(ctx, f.id)
	require.NoError(t, err)
	f.info.AgentContractFingerprint = "different"
	f.service.agentRuntime.mu.Lock()
	session := f.service.agentRuntime.sessions[f.id]
	f.service.agentRuntime.mu.Unlock()
	started, release := make(chan struct{}), make(chan struct{})
	var blocked atomic.Bool
	require.NoError(t, f.service.db.Callback().Query().After("gorm:query").Register("test:cached-verdict", func(*gorm.DB) {
		if blocked.CompareAndSwap(false, true) {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
	}))
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	ordinary := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		_, _, err := f.service.checkAgentCompatibility(ctx, f.id, session, false)
		ordinary <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("ordinary check did not read credentials")
	}
	forced := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		_, err := f.service.CheckAgentCompatibility(ctx, f.id)
		forced <- err
	}()
	require.Eventually(t, func() bool { return agentCheckWaiters() == 1 }, time.Second, time.Millisecond)
	close(release)
	require.NoError(t, <-ordinary)
	require.ErrorIs(t, <-forced, ErrAgentUpdateRequired, "explicit refresh must read the changed agent version")
	require.Equal(t, 2, f.count("GET /api/app-version"))
}
