// Source: docklet-next/issues/626
// Revision: 6dc802124ce20a9c53693493ca66a85d1820ef19
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original bounded producer resource-name regression tests for M41 S3.

package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

func TestSystemBackupCreateActivityResourceName(t *testing.T) {
	db, cfg := archiveFixture(t)
	acts := activity.NewActivityService(db, nil)
	var created *backuptypes.SystemBackupRun
	s := NewService(Dependencies{DB: db, Config: cfg, Activity: acts, Store: Store{CreateRun: func(_ context.Context, run *backuptypes.SystemBackupRun) error { created = run; return nil }}})
	run, work, err := s.startRun(t.Context(), user.Actor{}, backuptypes.SystemBackupTriggerManual, backuptypes.CreateSystemBackupRequest{})
	require.NoError(t, err)
	require.Same(t, run, created, "the real admission must reach its store")
	var stored activity.Activity
	require.NoError(t, db.First(&stored, "id = ?", run.ActivityID).Error)
	require.Equal(t, run.ID, *stored.ResourceID, "backup resource id must remain unchanged")
	require.NotNil(t, stored.ResourceName, "backup create must store the archive name")
	require.Equal(t, run.ID+".tar", *stored.ResourceName, "backup create resource name must be the derived archive name")
	_, err = acts.CompleteActivity(work, run.ActivityID, activitytypes.StatusSuccess, "backup.created", nil)
	require.NoError(t, err)
}

func TestSystemBackupDeleteActivityResourceName(t *testing.T) {
	for _, missingArchiveName := range []bool{false, true} {
		label := "stored-archive-name"
		if missingArchiveName {
			label = "no-stored-archive-name"
		}
		t.Run(label, func(t *testing.T) {
			db, cfg := archiveFixture(t)
			id := "system-example"
			name := id + ".tar"
			storedName := name
			if missingArchiveName {
				storedName = ""
			}
			require.NoError(t, os.WriteFile(filepath.Join(cfg.BackupDir, name), []byte("archive"), 0600))
			deleted := false
			s := NewService(Dependencies{DB: db, Config: cfg, Activity: activity.NewActivityService(db, nil), Store: Store{
				Run: func(context.Context, string) (*backuptypes.SystemBackupRun, error) {
					return &backuptypes.SystemBackupRun{ID: id, ArchiveName: storedName}, nil
				},
				DeleteRun: func(_ context.Context, got string) error { require.Equal(t, id, got); deleted = true; return nil },
			}})
			out, err := s.DeleteBackup(t.Context(), id)
			require.NoError(t, err)
			require.True(t, deleted)
			if !missingArchiveName {
				_, err := os.Stat(filepath.Join(cfg.BackupDir, name))
				require.ErrorIs(t, err, os.ErrNotExist)
			}
			var stored activity.Activity
			require.NoError(t, db.First(&stored, "id = ?", out.ActivityID).Error)
			require.Equal(t, activitytypes.StatusSuccess, stored.Status)
			require.Equal(t, id, *stored.ResourceID, "backup resource id must remain unchanged")
			require.NotNil(t, stored.ResourceName, "backup delete must store the archive name")
			require.Equal(t, name, *stored.ResourceName, "backup delete resource name must be the derived archive name")
		})
	}
}

func TestSystemBackupActivityNamePreservesStoreFailure(t *testing.T) {
	db, cfg := archiveFixture(t)
	acts := activity.NewActivityService(db, nil)
	failure := errors.New("store unavailable")
	s := NewService(Dependencies{DB: db, Config: cfg, Activity: acts, Store: Store{CreateRun: func(context.Context, *backuptypes.SystemBackupRun) error { return failure }}})
	_, _, err := s.startRun(t.Context(), user.Actor{}, backuptypes.SystemBackupTriggerManual, backuptypes.CreateSystemBackupRequest{})
	require.ErrorIs(t, err, failure)
	var stored activity.Activity
	require.NoError(t, db.First(&stored).Error)
	require.Equal(t, activitytypes.StatusFailed, stored.Status)
	require.NotNil(t, stored.ResourceName)
	require.Equal(t, *stored.ResourceID+".tar", *stored.ResourceName)
}
