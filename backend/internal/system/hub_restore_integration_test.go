//go:build postgres_integration

// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original PostgreSQL 18 offline and online restore round-trip for issue #32.

package system

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	backupservice "github.com/florianfinn/docklet-next/backend/internal/system/children/backup"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"
)

func TestHubRestorePostgres18RoundTrip(t *testing.T) {
	if _, err := exec.LookPath("pg_restore"); err != nil {
		t.Skip("PostgreSQL 18 client image required")
	}
	source, sourceURL := newMigratedHubBackupDBWithURL(t)
	require.NoError(t, source.Exec(`INSERT INTO users (id,created_at,username,password_hash,time_format) VALUES ('seed-user',NOW(),'example','hash','auto')`).Error)
	require.NoError(t, source.Exec(`INSERT INTO user_sessions (id,created_at,user_id,refresh_token_hash,last_used_at,expires_at) VALUES ('seed-session',NOW(),'seed-user','hash',NOW(),NOW()+INTERVAL '1 day')`).Error)
	require.NoError(t, source.Exec(`INSERT INTO settings (key,value) VALUES ('restore-example','before')`).Error)
	require.NoError(t, source.Exec(`INSERT INTO environment_update_jobs (id,created_at,status) VALUES ('seed-job',NOW(),'success')`).Error)
	require.NoError(t, source.Exec(`INSERT INTO activities (id,created_at,environment_id,type,status,started_at,ended_at,duration_ms,metadata) VALUES ('seed-activity',NOW(),'0','resource_action','success',NOW(),NOW(),0,'{}')`).Error)
	dir := t.TempDir()
	sourceSystem := &SystemService{db: source}
	sourceActivities := activity.NewActivityService(source, nil)
	sourceBackups := backupservice.NewService(backupservice.Dependencies{
		Store: sourceSystem.backupStoreInternal(), DB: source, Activity: sourceActivities,
		Config: &config.Config{DatabaseURL: sourceURL, BackupDir: dir},
	})
	run, err := sourceBackups.CreateBackup(t.Context(), usertypes.SystemUser, backuptypes.SystemBackupTriggerManual, backuptypes.CreateSystemBackupRequest{})
	require.NoError(t, err)
	require.Equal(t, backuptypes.SystemBackupStatusSucceeded, run.Status)
	targetURL := newEmptyHubRestoreDBURL(t)
	require.NoError(t, backupservice.OfflineRestore(t.Context(), &config.Config{DatabaseURL: targetURL, BackupDir: dir}, run.ID))
	require.NoError(t, backupservice.CheckRestoreMarker(dir))
	restarted, err := database.InitializeHub(t.Context(), targetURL, database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, restarted.Close()) })
	assertRestoredSeedInternal(t, restarted)
	var revoked *time.Time
	require.NoError(t, restarted.Raw("SELECT revoked_at FROM user_sessions WHERE id='seed-session'").Scan(&revoked).Error)
	require.NotNil(t, revoked)

	// The live path replaces a changed, initialized target and returns ready.
	require.NoError(t, restarted.Exec("UPDATE settings SET value='after' WHERE key='restore-example'").Error)
	targetSystem := &SystemService{db: restarted}
	targetActivities := activity.NewActivityService(restarted, nil)
	targetBackups := backupservice.NewService(backupservice.Dependencies{
		Store: targetSystem.backupStoreInternal(), DB: restarted, Activity: targetActivities,
		Config: &config.Config{DatabaseURL: targetURL, BackupDir: dir},
	})
	jobsRefreshed := make(chan struct{}, 1)
	targetBackups.SetRestoreSchedulerRefresh(func(context.Context) error {
		jobsRefreshed <- struct{}{}
		return nil
	})
	operation, err := targetBackups.StartRestore(t.Context(), usertypes.Actor{ID: "seed-user", Username: "example"}, "", run.ID, run.ID)
	require.NoError(t, err)
	require.NotEmpty(t, operation.ActivityID)
	require.Eventually(t, func() bool {
		var status string
		if restarted.Raw("SELECT status FROM activities WHERE id=?", operation.ActivityID).Scan(&status).Error != nil {
			return false
		}
		return status == "success"
	}, 30*time.Second, 100*time.Millisecond)
	require.NoError(t, backupservice.CheckRestoreMarker(dir))
	require.False(t, targetBackups.MaintenanceGate().Status().Active)
	select {
	case <-jobsRefreshed:
	default:
		t.Fatal("scheduler registrations were not refreshed")
	}
	assertRestoredSeedInternal(t, restarted)
}

func TestHubUploadStreamsThroughAuthorizedRoute(t *testing.T) {
	if _, err := exec.LookPath("pg_restore"); err != nil {
		t.Skip("PostgreSQL 18 client image required")
	}
	db, dsn := newMigratedHubBackupDBWithURL(t)
	dir := t.TempDir()
	system := &SystemService{db: db}
	activities := activity.NewActivityService(db, nil)
	service := backupservice.NewService(backupservice.Dependencies{
		Store: system.backupStoreInternal(), DB: db, Activity: activities,
		Config: &config.Config{DatabaseURL: dsn, BackupDir: dir},
	})
	run, err := service.CreateBackup(t.Context(), usertypes.SystemUser, backuptypes.SystemBackupTriggerManual, backuptypes.CreateSystemBackupRequest{})
	require.NoError(t, err)
	reader, _, _, err := service.OpenArchive(t.Context(), run.ID)
	require.NoError(t, err)
	archive, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	router := echo.New()
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("test", "1.0.0"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	backupservice.RegisterRoutes(api, backupservice.NewHandler(service, activities, t.Context()))
	req := httptest.NewRequest(http.MethodPost, "/api/backups/upload", bytes.NewReader(archive))
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "backupId")
	var count int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM system_backup_runs WHERE trigger='uploaded' AND status='succeeded'").Scan(&count).Error)
	require.Equal(t, 1, count)
}

func TestHubRestoreCommandFailureRollsBackAndResumes(t *testing.T) {
	if _, err := exec.LookPath("pg_restore"); err != nil {
		t.Skip("PostgreSQL 18 client image required")
	}
	db, dsn := newMigratedHubBackupDBWithURL(t)
	require.NoError(t, db.Exec("CREATE TABLE zz_late_data (id integer PRIMARY KEY, payload text NOT NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO zz_late_data SELECT n, repeat(md5(n::text), 8) FROM generate_series(1, 1000) AS n").Error)
	require.NoError(t, db.Exec("INSERT INTO settings (key,value) VALUES ('rollback-example','original')").Error)
	dir := t.TempDir()
	system := &SystemService{db: db}
	activities := activity.NewActivityService(db, nil)
	service := backupservice.NewService(backupservice.Dependencies{
		Store: system.backupStoreInternal(), DB: db, Activity: activities,
		Config: &config.Config{DatabaseURL: dsn, BackupDir: dir},
	})
	run, err := service.CreateBackup(t.Context(), usertypes.SystemUser, backuptypes.SystemBackupTriggerManual, backuptypes.CreateSystemBackupRequest{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("INSERT INTO zz_late_data (id, payload) VALUES (1001, 'newer')").Error)
	require.NoError(t, db.Exec("UPDATE settings SET value='newer' WHERE key='rollback-example'").Error)
	path := filepath.Join(dir, run.ArchiveName)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Greater(t, len(data), 1024)
	require.NoError(t, os.WriteFile(path, data[:len(data)-64], 0600))
	run.Size = int64(len(data) - 64)
	require.NoError(t, system.backupStoreInternal().SaveRun(t.Context(), run))
	operation, err := service.StartRestore(t.Context(), usertypes.Actor{ID: "operator", Username: "operator"}, "", run.ID, run.ID)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var status string
		if db.Raw("SELECT status FROM activities WHERE id=?", operation.ActivityID).Scan(&status).Error != nil {
			return false
		}
		return status == "failed"
	}, 30*time.Second, 100*time.Millisecond)
	require.Eventually(t, func() bool {
		return backupservice.CheckRestoreMarker(dir) == nil && !service.MaintenanceGate().Status().Active
	}, 10*time.Second, 50*time.Millisecond)
	var rows int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM zz_late_data").Scan(&rows).Error)
	require.Equal(t, 1001, rows)
	var value string
	require.NoError(t, db.Raw("SELECT value FROM settings WHERE key='rollback-example'").Scan(&value).Error)
	require.Equal(t, "newer", value)
}

func assertRestoredSeedInternal(t *testing.T, db *database.DB) {
	t.Helper()
	for _, query := range []string{
		"SELECT COUNT(*) FROM users WHERE id='seed-user' AND username='example'",
		"SELECT COUNT(*) FROM settings WHERE key='restore-example' AND value='before'",
		"SELECT COUNT(*) FROM environment_update_jobs WHERE id='seed-job' AND status='success'",
		"SELECT COUNT(*) FROM activities WHERE id='seed-activity' AND status='success'",
	} {
		var count int
		require.NoError(t, db.Raw(query).Scan(&count).Error)
		require.Equal(t, 1, count)
	}
	newest, err := database.LatestMigrationVersion()
	require.NoError(t, err)
	var current int64
	require.NoError(t, db.Raw("SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied=true").Scan(&current).Error)
	require.Equal(t, newest, current)
}

func newEmptyHubRestoreDBURL(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	require.NotEmpty(t, dsn)
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	pool, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	var random [8]byte
	_, err = rand.Read(random[:])
	require.NoError(t, err)
	name := "docklet_restore_" + hex.EncodeToString(random[:])
	require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
	t.Cleanup(func() { require.NoError(t, admin.Exec("DROP DATABASE "+name+" WITH (FORCE)").Error) })
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	parsed.Path = "/" + name
	return parsed.String()
}
