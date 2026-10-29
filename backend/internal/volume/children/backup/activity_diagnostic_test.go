// Source: docklet-next/issues/284
// Revision: 4a7f78d930c1ea765ae592feba903c528ebaad80
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original backup activity diagnostic boundary test.

package backup

import (
	"context"
	"encoding/json/v2"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/scheduler"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
)

const diagnosticSentinelInternal = "private-diagnostic-sentinel /srv/example"

// activityRowsInternal serialises every activity table row for a leak search.
func activityRowsInternal(t *testing.T, gormDB *gorm.DB) string {
	t.Helper()
	tables, err := gormDB.Migrator().GetTables()
	require.NoError(t, err)
	dump := ""
	for _, table := range tables {
		var rows []map[string]any
		require.NoError(t, gormDB.Table(table).Find(&rows).Error)
		encoded, err := json.Marshal(rows)
		require.NoError(t, err)
		dump += table + string(encoded)
	}
	return dump
}

func TestScheduledBackupActivityKeepsDiagnosticsPrivate(t *testing.T) {
	gormDB, err := gorm.Open(testsqlite.Dialector(t, "file:volume-backup-scheduled-diagnostic?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(&policies.VolumeBackupPolicy{}))
	require.NoError(t, gormDB.AutoMigrate(activity.Models()...))
	db := &database.DB{DB: gormDB}
	policy := &policies.VolumeBackupPolicy{VolumeName: "app-data", Enabled: true, Schedule: "0 0 2 * * *", RetentionCount: 7, LocalEnabled: true}
	require.NoError(t, gormDB.Create(policy).Error)
	gate := newVolumeAdmissionForTestInternal(t)
	engine := backup.NewEngine(t.Context(), gate, nil)
	t.Cleanup(func() { require.NoError(t, engine.Stop(context.WithoutCancel(t.Context()))) })
	service := NewService(Dependencies{
		DB:             db,
		Activity:       activity.NewActivityService(db, nil),
		Engine:         engine,
		AlreadyRunning: errors.New("a backup is already running for this volume"),
		Store: Store{Create: func(context.Context, *volume.Backup) error {
			return errors.New(diagnosticSentinelInternal)
		}},
	})
	jobScheduler := &volumeBackupPolicySchedulerInternal{jobs: make(map[string]scheduler.Job)}
	require.NoError(t, service.SetScheduler(t.Context(), jobScheduler, gate))
	service.RegisterJobsOnStartup(t.Context())
	job, registered := jobScheduler.jobs["volume-backup:"+policy.ID]
	require.True(t, registered)

	outcome, runErr := job.Run(t.Context())

	var backupActivity activity.Activity
	require.NoError(t, gormDB.Where("resource_type = ?", "volume_backup").First(&backupActivity).Error)
	require.Equal(t, activitytypes.StatusFailed, backupActivity.Status)
	require.NotNil(t, backupActivity.Error)
	require.Equal(t, "volume.backup.interrupted", *backupActivity.Error)
	require.NotContains(t, activityRowsInternal(t, gormDB), "private-diagnostic-sentinel")
	require.NotContains(t, outcome.Message, "private-diagnostic-sentinel")
	if runErr != nil {
		require.NotContains(t, runErr.Error(), "private-diagnostic-sentinel")
	}
}

func TestDurableBackupFailureKeepsDiagnosticsPrivate(t *testing.T) {
	gormDB, err := gorm.Open(testsqlite.Dialector(t, "file:volume-backup-durable-diagnostic?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(activity.Models()...))
	db := &database.DB{DB: gormDB}
	activities := activity.NewActivityService(db, nil)
	stored := ""
	service := NewService(Dependencies{
		DB:       db,
		Activity: activities,
		Store: Store{
			Run: func(_ context.Context, backupID string) (*volume.Backup, error) {
				return &volume.Backup{ID: backupID, VolumeName: "app-data"}, nil
			},
			Fail: func(_ context.Context, _ string, message string) error {
				stored = message
				return nil
			},
		},
	})
	actor := usertypes.SystemUser
	activityID, _ := activitylib.StartHandlerActivity(t.Context(), activities, "0", activitytypes.TypeResourceAction, "volume", "app-data", "app-data", &actor, "Creating backup", "Creating volume backup", database.JSON{"action": "create_volume_backup"}, false)
	require.NotEmpty(t, activityID)
	payload, err := json.Marshal(manualVolumeBackupInternal{Checkpoint: volumeBackupRecoveryInternal{BackupID: "backup-1"}, VolumeName: "app-data", ActivityID: activityID})
	require.NoError(t, err)

	require.NoError(t, service.failDurableBackupInternal(t.Context(), "run-1", payload, errors.New(diagnosticSentinelInternal)))

	require.Equal(t, "volume.backup.interrupted", stored)
	var record activity.Activity
	require.NoError(t, gormDB.Where("id = ?", activityID).First(&record).Error)
	require.Equal(t, activitytypes.StatusFailed, record.Status)
	require.NotContains(t, activityRowsInternal(t, gormDB), "private-diagnostic-sentinel")
}
