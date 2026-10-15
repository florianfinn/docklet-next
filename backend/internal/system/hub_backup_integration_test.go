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
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	backupstore "github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
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

func TestHubBackupRetentionAfterArchiveMigration(t *testing.T) {
	db := newMigratedHubBackupDB(t)
	store := (&SystemService{db: db}).backupStoreInternal()
	base := time.Now().UTC().Add(-time.Hour)
	for i, row := range []struct {
		id, policyID, status string
		archiveName          *string
	}{
		{"old", "daily", backuptypes.SystemBackupStatusSucceeded, new("old.dump")},
		{"without-archive", "daily", backuptypes.SystemBackupStatusSucceeded, nil},
		{"new", "daily", backuptypes.SystemBackupStatusSucceeded, new("new.dump")},
		{"failed", "daily", backuptypes.SystemBackupStatusFailed, new("failed.dump")},
		{"other-policy", "other", backuptypes.SystemBackupStatusSucceeded, new("other.dump")},
	} {
		require.NoError(t, db.Create(&SystemBackupRun{
			BaseModel: database.BaseModel{ID: row.id}, CreatedAt: base.Add(time.Duration(i) * time.Minute),
			Status: SystemBackupStatus(row.status), Trigger: SystemBackupTriggerManual,
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
	return db
}
