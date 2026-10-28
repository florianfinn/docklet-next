// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original persisted actor volume-intent settlement and no redispatch proof.

package backup

import (
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestVolumeRecoverySettlesActorsOnlyAfterWriterConfirmation(t *testing.T) {
	runtime := francistest.New(t)
	engine := NewEngine(t.Context(), nil, nil)
	engine.SetExecutionReady(func() bool { return false })
	require.NoError(t, engine.Register(runtime))
	francistest.Start(t, runtime)
	state := backup.DurableRunState{Command: backup.DurableRunCommand{RunID: "old-volume", Kind: "volume", Payload: []byte(`{"volumeName":"app-data"}`)}, Status: scheduler.Running, Started: true}
	require.NoError(t, engine.service.SetState(t.Context(), backupRunTypeInternal, "old-volume", state, nil))
	require.NoError(t, engine.SettleVolumeRuns(t.Context(), func(string) bool { return true }))
	var retained backup.DurableRunState
	require.NoError(t, engine.service.GetState(t.Context(), backupRunTypeInternal, "old-volume", &retained))
	require.Equal(t, scheduler.Running, retained.Status)
	require.NoError(t, engine.ReconcileDispatches(t.Context()))
	jobs, err := engine.service.ListJobs(t.Context(), backupRunTypeInternal, "old-volume")
	require.NoError(t, err)
	require.Empty(t, jobs, "fenced volume intents must never be redispatched")
	require.NoError(t, engine.SettleVolumeRuns(t.Context(), func(string) bool { return false }))
	require.NoError(t, engine.service.GetState(t.Context(), backupRunTypeInternal, "old-volume", &retained))
	require.Equal(t, scheduler.Failed, retained.Status)
	require.Equal(t, "volume.backup.interrupted", retained.Error)
}
