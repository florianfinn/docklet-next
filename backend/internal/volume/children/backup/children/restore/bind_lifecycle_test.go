// Source: docklet-next/issues/287
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package restore

import (
	"context"
	"errors"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBindFileRootCheckPrecedesWriting(t *testing.T) {
	l, run := lifecycleFixture(t)
	run.VolumeName = volume.BindKey("/srv/app.conf")
	require.NoError(t, l.DB.Save(run).Error)
	writes := 0
	var restarted []string
	l.Effects.PostSafety = func(ctx context.Context, r *Run, _ StartRequest) error {
		var stored Run
		require.NoError(t, l.DB.WithContext(ctx).First(&stored, "id = ?", r.ID).Error)
		require.Equal(t, "preparing", stored.Phase)
		require.Equal(t, "safety", stored.SafetyBackupID)
		return dto.NewProblem(422, "volume.bind.snapshot-kind-mismatch", nil)
	}
	l.Effects.Write = func(context.Context, *Run, StartRequest) error { writes++; return nil }
	l.Effects.Restart = func(_ context.Context, rows []volume.StoppedContainer, _ user.Actor) error {
		for _, row := range rows {
			restarted = append(restarted, row.ID)
		}
		return nil
	}
	err := l.Execute(t.Context(), run, StartRequest{VolumeName: run.VolumeName, Kind: "full"})
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, 422, p.Status)
	require.Equal(t, "volume.bind.snapshot-kind-mismatch", p.Code)
	require.Zero(t, writes)
	require.Equal(t, []string{"own"}, restarted)
	require.False(t, run.PendingRestart)
}
func TestBindFilePartialWriteKeepsResponsibility(t *testing.T) {
	l, run := lifecycleFixture(t)
	run.VolumeName = volume.BindKey("/srv/app.conf")
	require.NoError(t, l.DB.Save(run).Error)
	restarted := false
	l.Effects.Restart = func(context.Context, []volume.StoppedContainer, user.Actor) error { restarted = true; return nil }
	l.Effects.Write = func(context.Context, *Run, StartRequest) error { return errors.New("partial write") }
	err := l.Execute(t.Context(), run, StartRequest{VolumeName: run.VolumeName, Kind: "full"})
	require.Error(t, err)
	require.Equal(t, "volume.restore.failed-partial", run.ErrorCode)
	require.True(t, run.PendingRestart)
	require.Equal(t, "safety", run.SafetyBackupID)
	require.False(t, restarted)
}
func TestBindRestartIsSourceIndependent(t *testing.T) {
	l, current := lifecycleFixture(t)
	require.NoError(t, l.DB.Delete(current).Error)
	writing := stoppedFixture(t, l, "writing", "writing", "running", false, "partial")
	preparing := stoppedFixture(t, l, "preparing", "preparing", "running", false, "own")
	for _, row := range []*Run{writing, preparing} {
		row.VolumeName = volume.BindKey("/srv/removed")
		require.NoError(t, l.DB.Save(row).Error)
	}
	var restarted []string
	l.Effects.Prepare = func(context.Context, *Run, StartRequest) (func() error, error) {
		t.Fatal("restart inspected a source")
		return nil, nil
	}
	l.Effects.Restart = func(_ context.Context, set []volume.StoppedContainer, _ user.Actor) error {
		for _, c := range set {
			restarted = append(restarted, c.ID)
		}
		return nil
	}
	require.NoError(t, l.Reconcile(t.Context(), false, nil))
	require.Equal(t, []string{"own"}, restarted)
	require.NoError(t, l.DB.First(writing, "id = ?", "writing").Error)
	require.True(t, writing.PendingRestart)
	require.Equal(t, "volume.restore.interrupted", writing.ErrorCode)
}

func TestBindRestoreAsNewRefusesBeforeDocker(t *testing.T) {
	s := NewService(Dependencies{})
	for _, r := range []StartRequest{{VolumeName: volume.BindKey("/srv/app.conf"), Mode: "new", NewVolumeName: "copy"}, {VolumeName: "app", NewVolumeName: volume.BindKey("/srv/copy")}} {
		require.EqualError(t, s.ValidateAdmission(t.Context(), r), "volume.bind.unsupported")
	}
}
