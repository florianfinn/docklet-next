// Source: docklet-next/issues/81
// Revision: 79669bb43fa8ea72ea38bd55866b9b78b8a72a93
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original shared-check panic cleanup regression test.

package environment

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

func TestAgentCheckPanicReleasesWaitersAndAllowsRetry(t *testing.T) {
	f := newAgentFixture(t, "direct", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	env, err := f.service.GetEnvironmentByID(ctx, f.id)
	require.NoError(t, err)
	session, err := f.service.agentSessionFor(ctx, env)
	require.NoError(t, err)
	started, release := make(chan struct{}), make(chan struct{})
	var panicked atomic.Bool
	transport := f.service.httpClient.Transport
	f.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
		if !panicked.Swap(true) {
			close(started)
			select {
			case <-release:
				panic("test-only")
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		}
		return transport.RoundTrip(r)
	})
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	done := make(chan error, 2)
	request := func() {
		defer workers.Done()
		_, err := f.service.CheckAgentCompatibility(ctx, f.id)
		done <- err
	}
	workers.Add(1)
	go request()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("shared attempt did not reach the transport")
	}
	workers.Add(1)
	go request()
	require.Eventually(t, func() bool { return agentCheckWaiters() == 2 }, time.Second, time.Millisecond)
	close(release)
	for range 2 {
		select {
		case err := <-done:
			require.ErrorIs(t, err, remenv.ErrEnvironmentUnavailable, "a panicked attempt must fail closed for every caller")
		case <-time.After(time.Second):
			t.Fatal("a panicked attempt must release all waiters")
		}
	}
	session.checkMu.Lock()
	cleared := session.check == nil
	session.checkMu.Unlock()
	require.True(t, cleared, "a panicked attempt must clear the in-flight slot")
	_, err = f.service.CheckAgentCompatibility(ctx, f.id)
	require.NoError(t, err, "a new check must retry after the panicked attempt")
	require.Equal(t, 1, f.count("GET /api/app-version"))
}
