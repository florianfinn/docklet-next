// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original SQLite offline and online restore round-trip for issue #32.

package system

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	backupservice "github.com/florianfinn/docklet-next/backend/internal/system/children/backup"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"
)

func TestHubRestoreSQLiteRoundTrip(t *testing.T) {
	source, sourceURL := newMigratedHubBackupDBWithURL(t)
	require.NoError(t, source.Exec(`INSERT INTO users (id,created_at,username,password_hash,time_format) VALUES ('seed-user',(CAST((julianday('now') - 2440587.5) * 86400000000 AS INTEGER)),'example','hash','auto')`).Error)
	require.NoError(t, source.Exec(`INSERT INTO user_sessions (id,created_at,user_id,refresh_token_hash,last_used_at,expires_at) VALUES ('seed-session',(CAST((julianday('now') - 2440587.5) * 86400000000 AS INTEGER)),'seed-user','hash',(CAST((julianday('now') - 2440587.5) * 86400000000 AS INTEGER)),(CAST((julianday('now') - 2440587.5) * 86400000000 AS INTEGER)+86400000000))`).Error)
	require.NoError(t, source.Exec(`INSERT INTO settings (key,value) VALUES ('restore-example','before')`).Error)
	require.NoError(t, source.Exec(`INSERT INTO environment_update_jobs (id,created_at,status) VALUES ('seed-job',(CAST((julianday('now') - 2440587.5) * 86400000000 AS INTEGER)),'success')`).Error)
	require.NoError(t, source.Exec(`INSERT INTO activities (id,created_at,environment_id,type,status,started_at,ended_at,duration_ms,metadata) VALUES ('seed-activity',(CAST((julianday('now') - 2440587.5) * 86400000000 AS INTEGER)),'0','resource_action','success',(CAST((julianday('now') - 2440587.5) * 86400000000 AS INTEGER)),(CAST((julianday('now') - 2440587.5) * 86400000000 AS INTEGER)),0,'{}')`).Error)
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
	require.NoError(t, backupservice.CheckPendingRestore(&config.Config{DatabaseURL: targetURL}))
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
		return !targetBackups.MaintenanceGate().Status().Active && backupservice.CheckPendingRestore(&config.Config{DatabaseURL: targetURL}) == nil && len(jobsRefreshed) > 0
	}, 30*time.Second, 100*time.Millisecond)
	select {
	case <-jobsRefreshed:
	default:
		t.Fatal("scheduler registrations were not refreshed")
	}
	assertRestoredSeedInternal(t, restarted)
}

func TestHubUploadStreamsThroughAuthorizedRoute(t *testing.T) {
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
	return sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db"))
}

func TestHubBackupPolicyRouteCreatesFirstPolicyWithoutID(t *testing.T) {
	db, dsn := newMigratedHubBackupDBWithURL(t)
	system := &SystemService{db: db}
	service := backupservice.NewService(backupservice.Dependencies{Store: system.backupStoreInternal(), DB: db, Config: &config.Config{DatabaseURL: dsn, BackupDir: t.TempDir()}})
	router := echo.New()
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("test", "1.0.0"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	backupservice.RegisterRoutes(api, backupservice.NewHandler(service, nil, t.Context()))
	request := httptest.NewRequest(http.MethodPut, "/api/backups/policies", bytes.NewBufferString(`{"policies":[{"enabled":true,"schedule":"0 20 4 * * 1","retentionCount":7}]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var result backuptypes.SystemBackupPolicyCollection
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Len(t, result.Policies, 1)
	require.NotEmpty(t, result.Policies[0].ID)
	require.Equal(t, "0 20 4 * * 1", result.Policies[0].Schedule)
}
