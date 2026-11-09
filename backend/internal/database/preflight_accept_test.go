// Source: docklet-next/issues/361
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original accepted preflight states and temporary view lifecycle tests.

package database

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
)

// coldReleaseDatabase leaves a release database whose witness row exists only in
// the WAL of a producer that exited without Close or checkpoint.
func coldReleaseDatabase(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "release.db")
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestReleaseWALProducer$")
	command.Env = append(os.Environ(), "DOCKLET_TEST_RELEASE_WAL="+path)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	wal, err := os.Stat(path + "-wal")
	require.NoError(t, err)
	require.Positive(t, wal.Size())
	return path
}

func TestReleaseWALProducer(t *testing.T) {
	path := os.Getenv("DOCKLET_TEST_RELEASE_WAL")
	if path == "" {
		return
	}
	db, err := Initialize(t.Context(), sqlitestore.URL(path), MigrationOptions{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("INSERT INTO kv (key,value) VALUES ('preflight-witness','cold')").Error)
	os.Exit(0)
}

func TestPreflightAcceptsEmptyHeaderOnlyAndColdWALRelease(t *testing.T) {
	t.Run("zero bytes", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.db")
		require.NoError(t, os.WriteFile(path, nil, 0600))
		db, err := InitializeHub(t.Context(), sqlitestore.URL(path), MigrationOptions{})
		require.NoError(t, err)
		require.NoError(t, db.Close())
	})
	t.Run("header only", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "header.db")
		pool, err := sql.Open("sqlite", sqlitestore.URL(path))
		require.NoError(t, err)
		for _, statement := range []string{"PRAGMA journal_mode=DELETE", "CREATE TABLE scratch (id INTEGER)", "DROP TABLE scratch", "VACUUM"} {
			_, err = pool.Exec(statement)
			require.NoError(t, err)
		}
		require.NoError(t, pool.Close())
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Positive(t, info.Size())
		db, err := InitializeHub(t.Context(), sqlitestore.URL(path), MigrationOptions{})
		require.NoError(t, err)
		require.NoError(t, db.Close())
	})
	t.Run("cold WAL release replaces a stale view", func(t *testing.T) {
		path := coldReleaseDatabase(t)
		stale := preflightViewDir(path)
		require.NoError(t, os.MkdirAll(stale, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(stale, "inspect.db"), []byte("killed copy"), 0600))
		t.Cleanup(func() { _ = os.RemoveAll(stale) })
		db, err := InitializeHub(t.Context(), sqlitestore.URL(path), MigrationOptions{})
		require.NoError(t, err)
		var value string
		require.NoError(t, db.Raw("SELECT value FROM kv WHERE key='preflight-witness'").Scan(&value).Error)
		require.Equal(t, "cold", value, "WAL-only data is visible after the start")
		require.NoError(t, db.Close())
		require.NoDirExists(t, stale, "the preflight view is removed after inspection")
	})
}

func TestPreflightNamesMissingTemporarySpace(t *testing.T) {
	path := coldReleaseDatabase(t)
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	_, err := InitializeHub(t.Context(), sqlitestore.URL(path), MigrationOptions{})
	require.ErrorContains(t, err, "database preflight needs temporary space")
	require.ErrorContains(t, err, path)
}
