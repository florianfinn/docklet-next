//go:build postgres_integration

// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original startup reconciliation and failure-write regression test for issue #32.

package system

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	backupstore "github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	backupservice "github.com/florianfinn/docklet-next/backend/internal/system/children/backup"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"
)

func TestHubBackupFailureWritesAfterArchiveMigration(t *testing.T) {
	db := newMigratedHubBackupDB(t)
	service := &SystemService{db: db}
	store := service.backupStoreInternal()

	// Startup must succeed even when there are no abandoned rows to update.
	require.NoError(t, service.ReconcileInterruptedBackups(t.Context()))
	for _, id := range []string{"interrupted", "protected", "failed"} {
		require.NoError(t, store.CreateRun(t.Context(), &backuptypes.SystemBackupRun{
			ID: id, CreatedAt: time.Now().UTC(),
			Status: backuptypes.SystemBackupStatusRunning, Trigger: backuptypes.SystemBackupTriggerManual,
		}))
	}
	require.NoError(t, service.ReconcileInterruptedBackups(t.Context(), "protected"))
	require.NoError(t, store.FailRun(t.Context(), "protected", "private failure detail"))
	for _, tc := range []struct{ id, code string }{
		{"interrupted", "backup.interrupted"},
		{"protected", "backup.failed"},
		{"failed", "backup.interrupted"},
	} {
		var run SystemBackupRun
		require.NoError(t, db.First(&run, "id = ?", tc.id).Error)
		require.Equal(t, SystemBackupStatusFailed, run.Status)
		require.Equal(t, tc.code, run.ErrorCode)
	}
}

func TestHubBackupStartupSurvivesUnavailableStorage(t *testing.T) {
	db := newMigratedHubBackupDB(t)
	path := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0600))
	system := &SystemService{db: db}
	system.backup = backupservice.NewService(backupservice.Dependencies{
		Store:  system.backupStoreInternal(),
		Config: &config.Config{BackupDir: filepath.Join(path, "backups")},
	})
	require.NoError(t, system.ReconcileInterruptedBackups(t.Context()))
	storage := system.backup.Storage()
	require.False(t, storage.Available)
	require.Equal(t, backuptypes.SystemBackupStorageMissing, storage.ReasonCode)
}

func TestHubBackupRetentionAfterArchiveMigration(t *testing.T) {
	db := newMigratedHubBackupDB(t)
	store := (&SystemService{db: db}).backupStoreInternal()
	base := time.Now().UTC().Add(-time.Hour)
	for i, row := range []struct {
		id, policyID, status, trigger string
		archiveName                   *string
	}{
		{"old", "daily", backuptypes.SystemBackupStatusSucceeded, backuptypes.SystemBackupTriggerScheduled, new("old.dump")},
		{"without-archive", "daily", backuptypes.SystemBackupStatusSucceeded, backuptypes.SystemBackupTriggerScheduled, nil},
		{"new", "daily", backuptypes.SystemBackupStatusSucceeded, backuptypes.SystemBackupTriggerScheduled, new("new.dump")},
		{"manual", "daily", backuptypes.SystemBackupStatusSucceeded, backuptypes.SystemBackupTriggerManual, new("manual.dump")},
		{"failed", "daily", backuptypes.SystemBackupStatusFailed, backuptypes.SystemBackupTriggerScheduled, new("failed.dump")},
		{"other-policy", "other", backuptypes.SystemBackupStatusSucceeded, backuptypes.SystemBackupTriggerScheduled, new("other.dump")},
	} {
		require.NoError(t, db.Create(&SystemBackupRun{
			BaseModel: database.BaseModel{ID: row.id}, CreatedAt: base.Add(time.Duration(i) * time.Minute),
			Status: SystemBackupStatus(row.status), Trigger: SystemBackupTrigger(row.trigger),
			PolicyID: row.policyID, ArchiveName: row.archiveName, CopyState: backuptypes.SystemBackupCopyNone,
		}).Error)
	}
	ids, err := store.ExpiredRunIDs(t.Context(), "daily", 1)
	require.NoError(t, err)
	require.Equal(t, []string{"old"}, ids)
}

// Exercise the system backup store after all migrations so its SQL and mapped
// columns are checked by PostgreSQL against the installed schema.
func TestHubBackupStoreQueriesAfterArchiveMigration(t *testing.T) {
	db := newMigratedHubBackupDB(t)
	store := (&SystemService{db: db}).backupStoreInternal()
	// A new callback must be added to this schema exercise.
	require.Equal(t, 15, reflect.TypeOf(store).NumField())
	ctx := t.Context()
	run := &backuptypes.SystemBackupRun{
		ID: "archive", CreatedAt: time.Now().UTC(), Status: backuptypes.SystemBackupStatusRunning,
		Trigger: backuptypes.SystemBackupTriggerManual, PolicyID: "daily",
	}
	require.NoError(t, store.CreateRun(ctx, run))
	run.Status = backuptypes.SystemBackupStatusSucceeded
	run.ArchiveName = "archive.dump"
	require.NoError(t, store.SaveRun(ctx, run))
	_, err := store.Run(ctx, run.ID)
	require.NoError(t, err)
	params := pagination.QueryParams{SearchQuery: pagination.SearchQuery{Search: "archive"}, SortParams: pagination.SortParams{Sort: "createdAt", Order: pagination.SortDesc}, Params: pagination.Params{Limit: 10}}
	_, _, err = store.ListRuns(ctx, params)
	require.NoError(t, err)
	_, err = store.ExpiredRunIDs(ctx, "daily", 0)
	require.NoError(t, err)
	_, err = store.RunsByID(ctx, []string{run.ID})
	require.NoError(t, err)
	_, err = store.LatestPolicyRun(ctx, "daily")
	require.NoError(t, err)
	_, err = store.CountRuns(ctx)
	require.NoError(t, err)
	_, err = store.CountVolumeRepositoryRuns(ctx)
	require.NoError(t, err)
	_, _, err = store.ListHistory(ctx, params)
	require.NoError(t, err)
	require.NoError(t, store.FailRun(ctx, run.ID, "failure detail"))

	policy := SystemBackupPolicy{BaseModel: database.BaseModel{ID: "daily"}, Schedule: "0 0 3 * * *", RetentionCount: 1}
	require.NoError(t, db.Create(&policy).Error)
	_, err = store.Policies(ctx)
	require.NoError(t, err)
	_, err = store.Policy(ctx, policy.ID)
	require.NoError(t, err)
	require.NoError(t, store.ReconcilePolicies(ctx,
		[]backuptypes.UpdateSystemBackupPolicy{{ID: policy.ID, RetentionCount: 2}},
		func(_ context.Context, current *backuptypes.SystemBackupPolicy, update backuptypes.UpdateSystemBackupPolicy) error {
			current.RetentionCount = update.RetentionCount
			return nil
		},
		func(context.Context, string) {}, func(context.Context, *backuptypes.SystemBackupPolicy) {},
	))
	require.NoError(t, store.ReconcilePolicies(ctx, nil,
		func(context.Context, *backuptypes.SystemBackupPolicy, backuptypes.UpdateSystemBackupPolicy) error {
			return nil
		},
		func(context.Context, string) {}, func(context.Context, *backuptypes.SystemBackupPolicy) {},
	))
	require.NoError(t, store.DeleteRun(ctx, run.ID))
	recovery := backupstore.NewRecoveryKeyStore(db)
	_, err = recovery.Configured(ctx)
	require.NoError(t, err)
	_, err = recovery.Get(ctx)
	require.ErrorIs(t, err, backupstore.ErrRecoveryKeyNotConfigured)
	require.NoError(t, db.Save(&backupstore.SystemBackupRecoveryConfig{
		BaseModel: database.BaseModel{ID: backupstore.RecoveryKeyConfigID}, EncryptedRecoveryKey: "opaque",
	}).Error)
}

func newMigratedHubBackupDB(t *testing.T) *database.DB {
	db, _ := newMigratedHubBackupDBWithURL(t)
	return db
}

func TestHubArchiveCreateListDownloadAndGate(t *testing.T) {
	if os.Getenv("DOCKLET_BACKUP_INTEGRATION") != "1" {
		t.Skip("archive integration runs in the PostgreSQL 18 client image")
	}
	db, dsn := newMigratedHubBackupDBWithURL(t)
	require.NoError(t, db.Exec("CREATE TABLE backup_seed (value text NOT NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO backup_seed (value) VALUES ('example')").Error)
	system := &SystemService{db: db}
	dir := t.TempDir()
	activities := activity.NewActivityService(db, nil)
	service := backupservice.NewService(backupservice.Dependencies{
		Store: system.backupStoreInternal(), DB: db, Activity: activities,
		Config: &config.Config{DatabaseURL: dsn, BackupDir: dir},
	})
	gate, err := activities.StartGatedActivity(t.Context(), activity.GateRequest{
		EnvironmentID: "0", Operation: "existing:example", Scope: "system-backup", Type: activitytypes.TypeResourceAction,
	})
	require.NoError(t, err)
	_, err = service.StartBackup(t.Context(), usertypes.SystemUser, backuptypes.CreateSystemBackupRequest{})
	require.ErrorIs(t, err, backupservice.ErrSystemBackupAlreadyRunning)
	_, err = activities.CompleteActivity(gate.WorkContext, gate.Run.ID, activitytypes.StatusSuccess, "activity.success", nil)
	require.NoError(t, err)
	run, err := service.StartBackup(t.Context(), usertypes.Actor{ID: "operator-1", Username: "operator"}, backuptypes.CreateSystemBackupRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, run.ActivityID)
	require.Eventually(t, func() bool {
		latest, err := system.backupStoreInternal().Run(t.Context(), run.ID)
		return err == nil && latest.Status == backuptypes.SystemBackupStatusSucceeded
	}, 30*time.Second, 100*time.Millisecond)
	latest, err := system.backupStoreInternal().Run(t.Context(), run.ID)
	require.NoError(t, err)
	require.Equal(t, 18, latest.SourceServerMajor)
	require.GreaterOrEqual(t, latest.MigrationVersion, int64(8))
	params := pagination.QueryParams{SortParams: pagination.SortParams{Sort: "createdAt", Order: pagination.SortDesc}, Params: pagination.Params{Limit: 10}}
	listed, _, err := service.ListBackups(t.Context(), params)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	reader, size, name, err := service.OpenArchive(t.Context(), run.ID)
	require.NoError(t, err)
	require.Equal(t, latest.Size, size)
	require.Equal(t, latest.ArchiveName, name)
	var signature [5]byte
	_, err = reader.Read(signature[:])
	require.NoError(t, err)
	require.Equal(t, "PGDMP", string(signature[:]))
	require.NoError(t, reader.Close())
	command := exec.Command("pg_restore", "--list", filepath.Join(dir, name))
	command.Stdout, command.Stderr = io.Discard, io.Discard
	require.NoError(t, command.Run())
	detail, err := activities.GetActivityDetail(t.Context(), "0", run.ActivityID, 20)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusSuccess, detail.Activity.Status)
	require.Equal(t, "operator-1", detail.Activity.StartedBy.UserID)
	require.Empty(t, detail.Output)

	policy := SystemBackupPolicy{BaseModel: database.BaseModel{ID: "daily"}, Schedule: "0 0 3 * * *", RetentionCount: 1}
	require.NoError(t, db.Create(&policy).Error)
	_, err = service.UpdatePolicies(t.Context(), []backuptypes.UpdateSystemBackupPolicy{{
		ID: policy.ID, Enabled: true, Schedule: policy.Schedule, RetentionCount: policy.RetentionCount,
	}})
	require.NoError(t, err)
	manual, err := service.StartBackup(t.Context(), usertypes.SystemUser, backuptypes.CreateSystemBackupRequest{PolicyID: "daily"})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		stored, loadErr := system.backupStoreInternal().Run(t.Context(), manual.ID)
		return loadErr == nil && stored.Status == backuptypes.SystemBackupStatusSucceeded
	}, 30*time.Second, 100*time.Millisecond)
	first, err := service.CreateBackup(t.Context(), usertypes.SystemUser, backuptypes.SystemBackupTriggerScheduled,
		backuptypes.CreateSystemBackupRequest{PolicyID: "daily"})
	require.NoError(t, err)
	second, err := service.CreateBackup(t.Context(), usertypes.SystemUser, backuptypes.SystemBackupTriggerScheduled,
		backuptypes.CreateSystemBackupRequest{PolicyID: "daily"})
	require.NoError(t, err)
	_, err = system.backupStoreInternal().Run(t.Context(), first.ID)
	require.Error(t, err)
	_, err = system.backupStoreInternal().Run(t.Context(), manual.ID)
	require.NoError(t, err)
	secondFile, _, _, err := service.OpenArchive(t.Context(), second.ID)
	require.NoError(t, err)
	require.NoError(t, secondFile.Close())

	wrong, err := url.Parse(dsn)
	require.NoError(t, err)
	wrong.User = url.UserPassword("docklet", "wrong")
	failedService := backupservice.NewService(backupservice.Dependencies{
		Store: system.backupStoreInternal(), DB: db, Activity: activities,
		Config: &config.Config{DatabaseURL: wrong.String(), BackupDir: dir},
	})
	failed, err := failedService.StartBackup(t.Context(), usertypes.SystemUser, backuptypes.CreateSystemBackupRequest{})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		stored, loadErr := system.backupStoreInternal().Run(t.Context(), failed.ID)
		return loadErr == nil && stored.Status == backuptypes.SystemBackupStatusFailed
	}, 30*time.Second, 100*time.Millisecond)
	failedDetail, err := activities.GetActivityDetail(t.Context(), "0", failed.ActivityID, 20)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusFailed, failedDetail.Activity.Status)
	require.NotContains(t, failedDetail.Activity.LatestMessage, "wrong")
	require.Empty(t, failedDetail.Output)
	deleted, err := service.DeleteBackup(t.Context(), second.ID)
	require.NoError(t, err)
	require.Equal(t, string(activitytypes.StatusSuccess), deleted.Status)
	_, err = system.backupStoreInternal().Run(t.Context(), second.ID)
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(dir, second.ArchiveName))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func newMigratedHubBackupDBWithURL(t *testing.T) (*database.DB, string) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	require.NotEmpty(t, dsn, "DATABASE_URL must point to the test PostgreSQL service")
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adminSQL.Close()) })
	var suffix [8]byte
	_, err = rand.Read(suffix[:])
	require.NoError(t, err)
	name := "docklet_backup_store_" + hex.EncodeToString(suffix[:])
	require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
	t.Cleanup(func() { require.NoError(t, admin.Exec("DROP DATABASE "+name+" WITH (FORCE)").Error) })
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	parsed.Path = "/" + name
	db, err := database.Initialize(t.Context(), parsed.String(), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db, parsed.String()
}
