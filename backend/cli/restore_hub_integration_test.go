//go:build postgres_integration

// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original offline command integration test for issue #32.

package cli

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
)

func TestRestoreHubCommandIntoEmptyDatabase(t *testing.T) {
	for _, tool := range []string{"pg_dump", "pg_restore"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("PostgreSQL 18 client tool %s is unavailable", tool)
		}
		version, err := exec.Command(path, "--version").Output()
		require.NoError(t, err, "%s version probe failed", tool)
		require.Contains(t, string(version), "(PostgreSQL) 18.", "%s must be PostgreSQL 18", tool)
	}
	adminURL := os.Getenv("DATABASE_URL")
	require.NotEmpty(t, adminURL)
	admin, err := gorm.Open(postgres.Open(adminURL), &gorm.Config{})
	require.NoError(t, err)
	pool, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	createDatabase := func() string {
		t.Helper()
		var random [8]byte
		_, err := rand.Read(random[:])
		require.NoError(t, err)
		name := "docklet_cli_restore_" + hex.EncodeToString(random[:])
		require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
		t.Cleanup(func() { require.NoError(t, admin.Exec("DROP DATABASE "+name+" WITH (FORCE)").Error) })
		parsed, err := url.Parse(adminURL)
		require.NoError(t, err)
		parsed.Path = "/" + name
		return parsed.String()
	}
	sourceURL, targetURL := createDatabase(), createDatabase()
	source, err := database.Initialize(t.Context(), sourceURL, database.MigrationOptions{})
	require.NoError(t, err)
	defer source.Close()
	require.NoError(t, source.Exec("INSERT INTO settings (key,value) VALUES ('cli-restore-example','saved')").Error)
	dir := t.TempDir()
	const id = "cli-recovery"
	name, err := backuptypes.ArchiveNameForID(id)
	require.NoError(t, err)
	archive, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	require.NoError(t, err)
	parsed, err := url.Parse(sourceURL)
	require.NoError(t, err)
	password, _ := parsed.User.Password()
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	command := exec.CommandContext(t.Context(), "pg_dump", "--format=custom", "--no-owner", "--no-acl")
	command.Env = append(os.Environ(), "PGHOST="+parsed.Hostname(), "PGPORT="+port,
		"PGUSER="+parsed.User.Username(), "PGPASSWORD="+password,
		"PGDATABASE="+strings.TrimPrefix(parsed.Path, "/"), "PGSSLMODE=disable")
	command.Stdout, command.Stderr = archive, io.Discard
	require.NoError(t, command.Run(), "pg_dump could not create the archive; check the PostgreSQL 18 client and test server")
	require.NoError(t, archive.Close())
	t.Setenv("DATABASE_URL", targetURL)
	t.Setenv("DOCKLET_BACKUP_DIR", dir)
	rootCmd.SetArgs([]string{"restore-hub", "--backup-id", id})
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	require.NoError(t, rootCmd.ExecuteContext(t.Context()), "restore-hub failed; check the PostgreSQL 18 client, archive and test server")
	require.Contains(t, output.String(), "Hub archive restored")
	restarted, err := database.InitializeHub(t.Context(), targetURL, database.MigrationOptions{})
	require.NoError(t, err)
	defer restarted.Close()
	var value string
	require.NoError(t, restarted.Raw("SELECT value FROM settings WHERE key='cli-restore-example'").Scan(&value).Error)
	require.Equal(t, "saved", value)
}
