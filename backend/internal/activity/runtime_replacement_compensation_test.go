// Source: docklet-next/issues/366
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original observed replacement compensation inside its admitted run.

package activity_test

import (
	"sync"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/stability/stabilitytest"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestReplacementObservationFailureCompensatesInsideRun(t *testing.T) {
	daemon, registry := newContainerImageHTTP(t)
	clock := &replacementClock{Clock: stabilitytest.NewClock(), entered: make(chan struct{}), release: make(chan struct{})}
	unblock := sync.OnceFunc(func() { close(clock.release) })
	defer unblock()
	hub := activationApp(t, false, "", activationSetup{BeforeStart: func(f *runtimeServicesFixture, _ *activity.RuntimeHTTPService, _ *activity.RuntimeReceiver) {
		configureContainerImageFixture(t, f, daemon, registry)
		f.containers.ConfigureOperations(f.db.DB, f.store.RegistrationID, true, nil, nil)
		f.containers.UseObservationClock(clock)
	}})
	replacementImageRoutes(hub)
	status, body := activationCall(t, hub, "/api/environments/0/containers/"+runtimeContainerID+"/update", uuid.NewString(), "respond-async", []byte(`{"autoRestore":true}`))
	require.Equal(t, 202, status, string(body))
	run := activationRunID(t, body)
	replacementAwait(t, clock.entered)
	daemon.base.mu.Lock()
	candidate := daemon.base.containers[recreateNewID]
	candidate.Running = false
	daemon.base.containers[recreateNewID] = candidate
	daemon.base.mu.Unlock()
	unblock()
	snapshot, err := hub.services.waitLocal(run)
	require.NoError(t, err)
	require.NotNil(t, snapshot.ImageLifecycle)
	require.Equal(t, "update.unstable", snapshot.ImageLifecycle.Code, "candidate failure must retain its result, never run.busy self-refusal")
	require.NotNil(t, snapshot.ImageLifecycle.Recovery)
	require.Equal(t, "restored", snapshot.ImageLifecycle.Recovery.Outcome, "compensation must run inside R")
	old, found := daemon.base.copy(runtimeContainerID)
	require.True(t, found)
	require.True(t, old.Running)
	require.Equal(t, "app", old.Name)
	_, found = daemon.base.copy(recreateNewID)
	require.False(t, found, "compensation must remove the failed replacement")
}
