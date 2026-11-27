// Source: docklet-next/issues/675
// Revision: 89f95975c52d07523a80fdc1a6171fb9bd6816d4
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original shutdown result guard at the real scheduled backup producer.

package backup

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A backup that completed with a copy warning keeps its Partial result when shutdown cancels the executor.
func TestCompletedPartialBackupSurvivesShutdown(t *testing.T) {
	db, cfg := archiveFixture(t)
	require.NoError(t, db.Create(&backuptarget.Target{ID: "archive", Name: "Archive", Kind: "local", HostID: new("0"), Path: "/srv/archive"}).Error)
	policy := &dto.SystemBackupPolicy{ID: "daily", TargetID: "archive", Enabled: true, RetentionCount: 7}
	var saved dto.SystemBackupRun
	s := NewService(Dependencies{DB: db, Config: cfg, Activity: activity.NewActivityService(db, nil), Store: Store{
		Policy:        func(context.Context, string) (*dto.SystemBackupPolicy, error) { return policy, nil },
		CreateRun:     func(_ context.Context, r *dto.SystemBackupRun) error { saved = *r; return nil },
		SaveRun:       func(_ context.Context, r *dto.SystemBackupRun) error { saved = *r; return nil },
		CommitRunTx:   func(_ *gorm.DB, r *dto.SystemBackupRun, _ []string) error { saved = *r; return nil },
		ExpiredRunIDs: func(context.Context, string, int) ([]string, error) { return nil, nil },
	}})
	runtime := francistest.New(t)
	q := runs.New(runtime.Service(), time.UTC)
	require.NoError(t, q.Register(runtime))
	francistest.Start(t, runtime)
	completed := make(chan scheduler.Outcome, 1)
	q.SetExecutor(func(ctx context.Context, _ scheduler.Run) (scheduler.Outcome, error) {
		outcome, err := s.runScheduledBackup(ctx, policy.ID)
		completed <- outcome
		<-ctx.Done()
		return outcome, err
	}, nil)
	run, err := q.Submit(t.Context(), scheduler.Request{JobID: "system-backup:daily", Trigger: "scheduled"})
	require.NoError(t, err)
	require.NoError(t, q.Start(t.Context(), t.Context()))
	q.Activate()
	var outcome scheduler.Outcome
	select {
	case outcome = <-completed:
	case <-time.After(10 * time.Second):
		t.Fatal("backup did not finish")
	}
	stopCtx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	require.NoError(t, q.Stop(stopCtx))
	require.NoError(t, err)
	require.Equal(t, scheduler.Partial, outcome.Status)
	require.Equal(t, "backup.copy.mount-failed", outcome.Message)
	require.Equal(t, "backup.copy.mount-failed", outcome.Code)
	require.Equal(t, dto.SystemBackupStatusSucceeded, saved.Status)
	require.Equal(t, dto.SystemBackupCopyFailed, saved.CopyState)
	_, err = os.Stat(filepath.Join(cfg.BackupDir, saved.ArchiveName))
	require.NoError(t, err)
	var row activity.Activity
	require.NoError(t, db.First(&row, "id = ?", saved.ActivityID).Error)
	require.Equal(t, activitytypes.StatusSuccess, row.Status)
	require.Equal(t, "backup.copy.mount-failed", row.LatestMessage)
	stored, err := q.Get(t.Context(), "0", run.JobID, run.ID)
	require.NoError(t, err)
	require.Equal(t, scheduler.Partial, stored.Status, "the completed backup's warning result must survive shutdown")
	require.Equal(t, outcome.Code, stored.Outcome.Code)
}
