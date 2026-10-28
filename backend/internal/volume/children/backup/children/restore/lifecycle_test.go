// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original persisted-phase and own/inherited recovery failure proofs.

package restore

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func lifecycleFixture(t *testing.T) (*Lifecycle, *Run) {
	t.Helper()
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Run{}))
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sql.Close()) })
	l := &Lifecycle{DB: &database.DB{DB: db}}
	r := &Run{ID: "current", VolumeName: "app-data", Kind: "full", Mode: "replace", Phase: "preparing", Status: "running", StartedAt: time.Now().UTC()}
	require.NoError(t, db.Create(r).Error)
	l.Effects = LifecycleEffects{
		Prepare: func(context.Context, *Run, StartRequest) (func() error, error) {
			return func() error { return nil }, nil
		},
		Consumers: func(context.Context, string) ([]volume.StoppedContainer, error) {
			return []volume.StoppedContainer{{ID: "own", Name: "app"}}, nil
		},
		Stop: func(ctx context.Context, c volume.StoppedContainer, _ user.Actor) error {
			var stored Run
			require.NoError(t, db.First(&stored, "id = ?", r.ID).Error)
			require.Equal(t, []volume.StoppedContainer{c}, ownContainers(&stored))
			require.Equal(t, "preparing", stored.Phase)
			return nil
		},
		Restart: func(context.Context, []volume.StoppedContainer, user.Actor) error { return nil },
		Safety:  func(context.Context, string, user.Actor) (string, error) { return "safety", nil },
		Write: func(context.Context, *Run, StartRequest) error {
			var stored Run
			require.NoError(t, db.First(&stored, "id = ?", r.ID).Error)
			require.Equal(t, "writing", stored.Phase)
			require.Equal(t, "safety", stored.SafetyBackupID)
			return nil
		},
		CleanupNew: func(context.Context, *Run) error { return nil },
	}
	return l, r
}
func stoppedFixture(t *testing.T, l *Lifecycle, id, phase, status string, pending bool, consumer string) *Run {
	t.Helper()
	payload, err := json.Marshal([]volume.StoppedContainer{{ID: consumer, Name: consumer}})
	require.NoError(t, err)
	row := &Run{ID: id, VolumeName: "app-data", Kind: "full", Mode: "replace", Phase: phase, Status: status, PendingRestart: pending, StoppedContainers: new(string(payload)), StartedAt: time.Now().UTC()}
	require.NoError(t, l.DB.Create(row).Error)
	return row
}
func TestLifecycleFailuresPersistBeforeEffects(t *testing.T) {
	for _, stage := range []string{"prepare", "stop", "safety", "write", "restart", "cleanup"} {
		t.Run(stage, func(t *testing.T) {
			l, r := lifecycleFixture(t)
			var restarted []string
			writes := 0
			cleaned := false
			fenced := false
			l.Effects.Restart = func(_ context.Context, set []volume.StoppedContainer, _ user.Actor) error {
				require.True(t, cleaned, "helpers must be confirmed gone before restart")
				for _, c := range set {
					restarted = append(restarted, c.ID)
				}
				if stage == "restart" {
					return errors.New("restart denied")
				}
				return nil
			}
			l.Effects.Prepare = func(context.Context, *Run, StartRequest) (func() error, error) {
				cleanup := func() error {
					cleaned = true
					if stage == "cleanup" {
						return errors.New("inspect unavailable")
					}
					return nil
				}
				if stage == "prepare" {
					return cleanup, errors.New("scratch preparation failed")
				}
				return cleanup, nil
			}
			l.Effects.Fence = func(context.Context, string) { fenced = true }
			originalStop := l.Effects.Stop
			l.Effects.Stop = func(ctx context.Context, c volume.StoppedContainer, u user.Actor) error {
				require.NoError(t, originalStop(ctx, c, u))
				if stage == "stop" {
					return errors.New("stop failed")
				}
				return nil
			}
			l.Effects.Safety = func(context.Context, string, user.Actor) (string, error) {
				if stage == "safety" {
					return "", errors.New("snapshot failed")
				}
				return "safety", nil
			}
			originalWrite := l.Effects.Write
			l.Effects.Write = func(ctx context.Context, row *Run, r StartRequest) error {
				writes++
				require.NoError(t, originalWrite(ctx, row, r))
				if stage == "write" {
					return errors.New("partial write")
				}
				return nil
			}
			err := l.Execute(t.Context(), r, StartRequest{})
			var stored Run
			require.NoError(t, l.DB.First(&stored, "id = ?", r.ID).Error)
			switch stage {
			case "cleanup":
				require.Error(t, err)
				require.True(t, fenced)
				require.Equal(t, "running", stored.Status)
				require.Empty(t, restarted)
			case "restart":
				require.NoError(t, err)
				require.Equal(t, "partial", stored.Status)
				require.Equal(t, "done", stored.Phase)
				require.Equal(t, "volume.restore.restart-failed", stored.ErrorCode)
			case "write":
				require.Error(t, err)
				require.True(t, stored.PendingRestart)
				require.Equal(t, "volume.restore.failed-partial", stored.ErrorCode)
				require.Empty(t, restarted)
			default:
				require.Error(t, err)
				require.Equal(t, "failed", stored.Status)
				require.False(t, stored.PendingRestart)
				require.Zero(t, writes)
				if stage == "prepare" {
					require.Empty(t, restarted)
				} else {
					require.Equal(t, []string{"own"}, restarted)
				}
			}
		})
	}
}
func TestLifecycleOwnAndAllInheritedObligations(t *testing.T) {
	l, r := lifecycleFixture(t)
	stoppedFixture(t, l, "old-a", "writing", "failed", true, "inherited-a")
	stoppedFixture(t, l, "old-b", "writing", "failed", true, "inherited-b")
	var restarted []string
	l.Effects.Restart = func(_ context.Context, set []volume.StoppedContainer, _ user.Actor) error {
		for _, c := range set {
			restarted = append(restarted, c.ID)
		}
		return nil
	}
	// A recovery that fails before writing restarts only what it stopped.
	l.Effects.Safety = func(context.Context, string, user.Actor) (string, error) { return "", errors.New("safety failed") }
	require.Error(t, l.Execute(t.Context(), r, StartRequest{}))
	require.Equal(t, []string{"own"}, restarted)
	require.NoError(t, l.Reconcile(t.Context(), false, nil))
	for _, id := range []string{"old-a", "old-b"} {
		var row Run
		require.NoError(t, l.DB.First(&row, "id = ?", id).Error)
		require.True(t, row.PendingRestart)
	}
	// A later successful recovery settles every retained row, including older ones.
	r = &Run{ID: "recovery", VolumeName: "app-data", Mode: "replace", Kind: "full", Phase: "preparing", Status: "running", StartedAt: time.Now().UTC()}
	require.NoError(t, l.DB.Create(r).Error)
	l.Effects.Stop = func(context.Context, volume.StoppedContainer, user.Actor) error { return nil }
	l.Effects.Safety = func(context.Context, string, user.Actor) (string, error) { return "safety", nil }
	l.Effects.Write = func(context.Context, *Run, StartRequest) error { return nil }
	restarted = nil
	require.NoError(t, l.Execute(t.Context(), r, StartRequest{}))
	require.ElementsMatch(t, []string{"own", "inherited-a", "inherited-b"}, restarted)
	var pending int64
	require.NoError(t, l.DB.Model(&Run{}).Where("pending_restart = ?", true).Count(&pending).Error)
	require.Zero(t, pending)
	require.Equal(t, []volume.StoppedContainer{{ID: "own", Name: "app"}}, ownContainers(r))
}
func TestReconcileNeverWritesOrRestartsInterruptedWrites(t *testing.T) {
	l, _ := lifecycleFixture(t)
	stoppedFixture(t, l, "writing", "writing", "running", false, "partial")
	stoppedFixture(t, l, "preparing", "preparing", "running", false, "own")
	var restarted []string
	l.Effects.Write = func(context.Context, *Run, StartRequest) error { t.Fatal("startup replayed a write"); return nil }
	l.Effects.Restart = func(_ context.Context, set []volume.StoppedContainer, _ user.Actor) error {
		for _, c := range set {
			restarted = append(restarted, c.ID)
		}
		return nil
	}
	require.NoError(t, l.Reconcile(t.Context(), false, nil))
	require.Equal(t, []string{"own"}, restarted)
	var writing Run
	require.NoError(t, l.DB.First(&writing, "id = ?", "writing").Error)
	require.True(t, writing.PendingRestart)
	require.Equal(t, "volume.restore.interrupted", writing.ErrorCode)
}
func TestRestoredHistoryCannotAuthorizeDockerEffects(t *testing.T) {
	l, _ := lifecycleFixture(t)
	stoppedFixture(t, l, "failed-pending", "writing", "failed", true, "old-state")
	stoppedFixture(t, l, "old-preparing", "preparing", "running", false, "old-consumer")
	l.Effects.Restart = func(context.Context, []volume.StoppedContainer, user.Actor) error {
		t.Fatal("restored archive restarted a consumer")
		return nil
	}
	l.Effects.CleanupNew = func(context.Context, *Run) error { t.Fatal("restored archive removed a volume"); return nil }
	require.NoError(t, l.Reconcile(t.Context(), true, nil))
	var pending int64
	require.NoError(t, l.DB.Model(&Run{}).Where("pending_restart = ?", true).Count(&pending).Error)
	require.Zero(t, pending)
}
func TestScratchENOSPCKeepsTargetUntouched(t *testing.T) {
	l, r := lifecycleFixture(t)
	removed := false
	l.Effects.Prepare = func(context.Context, *Run, StartRequest) (func() error, error) {
		return func() error { removed = true; return nil }, scratchProblem(errors.New("docker archive: no space left on device"))
	}
	l.Effects.Consumers = func(context.Context, string) ([]volume.StoppedContainer, error) {
		t.Fatal("consumers read after scratch failure")
		return nil, nil
	}
	l.Effects.Write = func(context.Context, *Run, StartRequest) error {
		t.Fatal("target written after scratch failure")
		return nil
	}
	err := l.Execute(t.Context(), r, StartRequest{Kind: "upload"})
	require.Error(t, err)
	require.True(t, removed)
	require.Equal(t, "preparing", r.Phase)
	require.Equal(t, "volume.backup.upload-invalid", r.ErrorCode)
	var executeProblem *dto.Problem
	require.ErrorAs(t, err, &executeProblem)
	require.Equal(t, "insufficient-space", executeProblem.Reason)
	// The public refusal retains a safe reason at the preparation boundary.
	var p *dto.Problem
	require.ErrorAs(t, scratchProblem(errors.New("tar: no space left on device")), &p)
	require.Equal(t, "insufficient-space", p.Reason)
}

func TestAcceptedRestoreIsOwnedByMaintenanceDrain(t *testing.T) {
	for _, phase := range []string{"preparing", "writing"} {
		t.Run(phase, func(t *testing.T) {
			l, _ := lifecycleFixture(t)
			require.NoError(t, l.DB.AutoMigrate(activity.Models()...))
			l.Activity = activity.NewActivityService(l.DB, nil)
			cancelled := make(chan struct{})
			parked := make(chan struct{})
			released := make(chan struct{})
			done := make(chan struct{})
			l.Effects.Validate = func(context.Context, StartRequest) error { return nil }
			l.Effects.Admit = func(ctx context.Context, _, _ string) (context.Context, func(), error) {
				return ctx, func() { close(done) }, nil
			}
			prepare := l.Effects.Prepare
			if phase == "preparing" {
				l.Effects.Prepare = func(ctx context.Context, r *Run, q StartRequest) (func() error, error) {
					close(parked)
					<-ctx.Done()
					close(cancelled)
					<-released
					return func() error { return nil }, ctx.Err()
				}
			} else {
				l.Effects.Prepare = prepare
				l.Effects.Stop = func(context.Context, volume.StoppedContainer, user.Actor) error { return nil }
				l.Effects.Write = func(ctx context.Context, _ *Run, _ StartRequest) error {
					close(parked)
					<-ctx.Done()
					close(cancelled)
					<-released
					return ctx.Err()
				}
			}
			owned := filepath.Join(t.TempDir(), "restore-inputs", phase, "input")
			require.NoError(t, os.MkdirAll(filepath.Dir(owned), 0700))
			require.NoError(t, os.WriteFile(owned, []byte("owned"), 0600))
			accepted, err := l.Start(t.Context(), StartRequest{EnvironmentID: "0", VolumeName: "app-data", Kind: "upload", InputPath: owned, User: user.SystemUser})
			require.NoError(t, err)
			require.NotEmpty(t, accepted.ActivityID)
			require.NotEmpty(t, accepted.RestoreID)
			<-parked
			_, err = os.Stat(owned)
			require.NoError(t, err, "handler return must not clean up accepted worker input")
			drained := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				drained <- l.Activity.CancelAndDrain(ctx, "")
			}()
			select {
			case <-cancelled:
			case err := <-drained:
				t.Fatalf("drain returned before cancelling its worker: %v", err)
			}
			select {
			case err := <-drained:
				t.Fatalf("drain returned before terminal worker persistence: %v", err)
			default:
			}
			close(released)
			require.NoError(t, <-drained)
			<-done
			var row Run
			require.NoError(t, l.DB.First(&row, "id = ?", accepted.RestoreID).Error)
			require.Equal(t, "failed", row.Status)
			require.Equal(t, phase, row.Phase)
			require.Equal(t, phase == "writing", row.PendingRestart)
			_, err = os.Stat(owned)
			require.True(t, os.IsNotExist(err))
		})
	}
}

func TestNewRestoreDoesNotSettleSourceRecoveryObligations(t *testing.T) {
	l, r := lifecycleFixture(t)
	stoppedFixture(t, l, "source-pending", "writing", "failed", true, "source-consumer")
	r.Mode = "new"
	r.NewVolumeName = "new-data"
	l.Effects.Consumers = func(context.Context, string) ([]volume.StoppedContainer, error) {
		t.Fatal("new restore stopped source consumers")
		return nil, nil
	}
	l.Effects.Safety = func(context.Context, string, user.Actor) (string, error) {
		t.Fatal("new restore backed up source")
		return "", nil
	}
	l.Effects.Write = func(context.Context, *Run, StartRequest) error { return nil }
	require.NoError(t, l.Execute(t.Context(), r, StartRequest{Mode: "new"}))
	var source Run
	require.NoError(t, l.DB.First(&source, "id = ?", "source-pending").Error)
	require.True(t, source.PendingRestart)
}
