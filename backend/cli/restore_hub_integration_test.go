// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original coordinated SQLite backup and paired recovery for issue #32.

package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	backupservice "github.com/florianfinn/docklet-next/backend/internal/system/children/backup"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/francis"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

func TestRestoreHubCommandIntoEmptyDatabase(t *testing.T) {
	sourceURL := sqlitestore.URL(filepath.Join(t.TempDir(), "source.db"))
	targetURL := sqlitestore.URL(filepath.Join(t.TempDir(), "target.db"))
	source, err := database.InitializeHub(t.Context(), sourceURL, database.MigrationOptions{})
	require.NoError(t, err)
	defer source.Close()
	require.NoError(t, francis.InitializeStore(t.Context(), sourceURL))
	require.NoError(t, source.Exec("INSERT INTO settings(key,value) VALUES('cli-restore-example','saved')").Error)
	dir := t.TempDir()
	service := backupservice.NewService(backupservice.Dependencies{DB: source, Activity: activity.NewActivityService(source, nil), Config: &config.Config{DatabaseURL: sourceURL, BackupDir: dir}, Store: backupservice.Store{
		CreateRun: func(context.Context, *backuptypes.SystemBackupRun) error { return nil }, SaveRun: func(context.Context, *backuptypes.SystemBackupRun) error { return nil },
	}})
	run, err := service.CreateBackup(t.Context(), usertypes.SystemUser, backuptypes.SystemBackupTriggerManual, backuptypes.CreateSystemBackupRequest{})
	require.NoError(t, err)
	t.Setenv("DATABASE_URL", targetURL)
	t.Setenv("DOCKLET_BACKUP_DIR", dir)
	t.Setenv("DOCKER_CONFIG", filepath.Join(t.TempDir(), "docker-config"))
	t.Setenv("PROJECTS_DIRECTORY", filepath.Join(t.TempDir(), "projects"))
	rootCmd.SetArgs([]string{"restore-hub", "--backup-id", run.ID})
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	t.Cleanup(func() { rootCmd.SetOut(nil); rootCmd.SetArgs(nil) })
	require.NoError(t, rootCmd.ExecuteContext(t.Context()))
	require.Contains(t, output.String(), "Hub archive restored")
	restarted, err := database.InitializeHub(t.Context(), targetURL, database.MigrationOptions{})
	require.NoError(t, err)
	defer restarted.Close()
	var value string
	require.NoError(t, restarted.Raw("SELECT value FROM settings WHERE key='cli-restore-example'").Scan(&value).Error)
	require.Equal(t, "saved", value)
}

func TestD11RestoreHubValidationPrintsErrorWithoutUsage(t *testing.T) {
	restoreHubCmd.SetContext(t.Context())
	t.Cleanup(func() { restoreHubCmd.SetContext(nil) })
	rootCmd.SetArgs([]string{"restore-hub", "--backup-id", "missing-example"})
	t.Setenv("DATABASE_URL", sqlitestore.URL(filepath.Join(t.TempDir(), "target.db")))
	t.Setenv("DOCKLET_BACKUP_DIR", t.TempDir())
	t.Setenv("DOCKER_CONFIG", filepath.Join(t.TempDir(), "docker-config"))
	t.Setenv("PROJECTS_DIRECTORY", filepath.Join(t.TempDir(), "projects"))
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)
	t.Cleanup(func() { rootCmd.SetOut(nil); rootCmd.SetErr(nil); rootCmd.SetArgs(nil) })
	require.Error(t, rootCmd.ExecuteContext(t.Context()))
	require.Contains(t, output.String(), "backup.invalid_archive")
	require.NotContains(t, output.String(), "Usage:")
	require.NotContains(t, output.String(), "Flags:")
}
