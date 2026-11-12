// Source: docklet-next/issues/289
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package backup

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestHubCopyFailureUsesPruningPartialOutcome(t *testing.T) {
	db, cfg := archiveFixture(t)
	require.NoError(t, db.Create(&backuptarget.Target{ID: "archive", Name: "Archive", Kind: "local", HostID: new("0"), Path: "/srv/archive"}).Error)
	policy := &dto.SystemBackupPolicy{ID: "daily", TargetID: "archive", Enabled: true, RetentionCount: 7}
	var saved dto.SystemBackupRun
	s := NewService(Dependencies{DB: db, Config: cfg, Activity: activity.NewActivityService(db, nil), Store: Store{
		Policy:        func(context.Context, string) (*dto.SystemBackupPolicy, error) { return policy, nil },
		CreateRun:     func(_ context.Context, r *dto.SystemBackupRun) error { saved = *r; return nil },
		SaveRun:       func(_ context.Context, r *dto.SystemBackupRun) error { saved = *r; return nil },
		ExpiredRunIDs: func(context.Context, string, int) ([]string, error) { return nil, nil },
	}})
	outcome, err := s.runScheduledBackup(t.Context(), policy.ID)
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
}
