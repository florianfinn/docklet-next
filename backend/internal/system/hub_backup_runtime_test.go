// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original coordinated SQLite backup and paired recovery for issue #32.

package system

import (
	"context"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	backupservice "github.com/florianfinn/docklet-next/backend/internal/system/children/backup"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	schedulertypes "github.com/florianfinn/docklet-next/types/scheduler"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

func TestScheduledSQLiteBackupAndLiveRestoreRestartActors(t *testing.T) {
	db, dsn := newMigratedHubBackupDBWithURL(t)
	runtime := francistest.New(t, dsn)
	coordinator := runs.New(runtime.Service(), time.UTC)
	require.NoError(t, coordinator.Register(runtime))
	francistest.Start(t, runtime)
	activities := activity.NewActivityService(db, nil)
	cfg := &config.Config{DatabaseURL: dsn, BackupDir: t.TempDir()}
	backups := backupservice.NewService(backupservice.Dependencies{Store: (&SystemService{db: db}).backupStoreInternal(), DB: db, Activity: activities, Config: cfg, Coordinator: coordinator, Runtime: runtime})
	require.NoError(t, db.Create(&SystemBackupPolicy{BaseModel: database.BaseModel{ID: "daily"}, Enabled: true, Schedule: "0 0 3 * * *", RetentionCount: 1}).Error)
	archiveID := make(chan string, 1)
	coordinator.SetExecutor(func(ctx context.Context, _ schedulertypes.Run) (schedulertypes.Outcome, error) {
		run, err := backups.CreateBackup(ctx, usertypes.SystemUser, backuptypes.SystemBackupTriggerScheduled, backuptypes.CreateSystemBackupRequest{PolicyID: "daily"})
		if err == nil {
			archiveID <- run.ID
		}
		outcome := schedulertypes.Outcome{Status: schedulertypes.Succeeded}
		if run != nil {
			outcome.ActivityID = run.ActivityID
		}
		return outcome, err
	}, nil)
	require.NoError(t, coordinator.Start(t.Context(), t.Context()))
	coordinator.Activate()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
		defer cancel()
		require.NoError(t, coordinator.Stop(ctx))
	})
	job, err := coordinator.Submit(t.Context(), schedulertypes.Request{JobID: "system-backup:daily", EnvironmentID: "0"})
	require.NoError(t, err)
	var id string
	select {
	case id = <-archiveID:
	case <-time.After(15 * time.Second):
		t.Fatal("scheduled snapshot did not finish")
	}
	require.Eventually(t, func() bool {
		run, err := coordinator.Get(t.Context(), "0", job.JobID, job.ID)
		return err == nil && run.Status == schedulertypes.Succeeded
	}, 5*time.Second, 20*time.Millisecond)
	require.NoError(t, db.Exec("INSERT INTO settings(key,value) VALUES('after-snapshot','discard')").Error)
	refreshed := make(chan struct{}, 1)
	backups.SetRestoreSchedulerRefresh(func(context.Context) error { refreshed <- struct{}{}; return nil })
	_, err = backups.StartRestore(t.Context(), usertypes.SystemUser, "", id, id)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(refreshed) > 0 && !backups.MaintenanceGate().Status().Active }, 15*time.Second, 50*time.Millisecond)
	var count int64
	require.NoError(t, db.Table("settings").Where("key='after-snapshot'").Count(&count).Error)
	require.Zero(t, count)
	records, err := coordinator.Records(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, records)
	require.True(t, coordinator.Active())
	store, err := runtime.Store()
	require.NoError(t, err)
	require.NoError(t, store.Table("arcane_francis_hosts").Count(&count).Error)
	require.EqualValues(t, 1, count)
}
