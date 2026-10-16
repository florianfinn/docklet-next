// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: File policy, network-storage refusal and real WAL/constraint/storage errors.

package sqlitestore

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestFileURLPolicyAndAliases(t *testing.T) {
	for _, raw := range []string{"", ":memory:", "file::memory:", "postgres://example.invalid/db", "file://example.invalid/data.db", "file:a.db?mode=ro", "file:a.db?mode=memory", "file:a.db?cache=shared", "file:a.db?immutable=1", "file:a.db?vfs=unix-dotfile", "file:a.db?_pragma=foreign_keys(0)", "file:a.db?_txlock=deferred", "file:a.db?_inttotime=0", "file:a.db?_time_integer_format=unix_milli"} {
		_, err := Path(raw)
		require.Error(t, err, raw)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "with space #.db")
	resolved, err := Path(URL(path))
	require.NoError(t, err)
	require.Equal(t, path, resolved)
	require.Equal(t, filepath.Join(dir, "with space #.francis.db"), FrancisPath(path))
	require.Equal(t, filepath.Join(dir, "extensionless.francis.db"), FrancisPath(filepath.Join(dir, "extensionless")))
	pool, err := Open(t.Context(), path, true, false)
	require.NoError(t, err)
	require.NoError(t, pool.Close())
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	symlink := filepath.Join(dir, "symlink.db")
	require.NoError(t, os.Symlink(path, symlink))
	_, err = Path(URL(symlink))
	require.Error(t, err)
	alias := filepath.Join(dir, "hardlink.db")
	require.NoError(t, os.Link(path, alias))
	_, err = Path(URL(alias))
	require.Error(t, err)
	_, err = Path(URL(path))
	require.Error(t, err)
}
func TestNetworkFilesystemRefusalIsFailClosed(t *testing.T) {
	require.ErrorContains(t, checkLocalDirectory("unused", func(string) (bool, error) { return true, nil }), "local persistent")
	require.ErrorContains(t, checkLocalDirectory("unused", func(string) (bool, error) { return false, errors.New("statfs failed") }), "verify local")
	require.NoError(t, checkLocalDirectory("unused", func(string) (bool, error) { return false, nil }))
}
func TestWALCheckpointReadOnlyAndStorageErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.db")
	pool, err := Open(t.Context(), path, false, false)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec("CREATE TABLE sample(id INTEGER PRIMARY KEY,data TEXT)")
	require.NoError(t, err)
	_, err = pool.Exec("INSERT INTO sample(data) VALUES('kept')")
	require.NoError(t, err)
	reader, err := Open(t.Context(), path, false, true)
	require.NoError(t, err)
	defer reader.Close()
	_, err = reader.Exec("INSERT INTO sample(data) VALUES('forbidden')")
	require.Error(t, err)
	var busy, frames, checkpointed int
	require.NoError(t, pool.QueryRow("PRAGMA wal_checkpoint(PASSIVE)").Scan(&busy, &frames, &checkpointed))
	require.Zero(t, busy)
	require.Equal(t, frames, checkpointed)
	require.NoError(t, pool.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &frames, &checkpointed))
	require.Zero(t, busy)
	require.Zero(t, frames)
	var pages int
	require.NoError(t, pool.QueryRow("PRAGMA page_count").Scan(&pages))
	_, err = pool.Exec("PRAGMA max_page_count=" + fmt.Sprint(pages))
	require.NoError(t, err)
	_, err = pool.Exec("INSERT INTO sample(data) VALUES(zeroblob(1048576))")
	require.ErrorContains(t, err, "full")
	var count int
	require.NoError(t, pool.QueryRow("SELECT count(*) FROM sample").Scan(&count))
	require.Equal(t, 1, count)
	bad := filepath.Join(t.TempDir(), "corrupt.db")
	require.NoError(t, os.WriteFile(bad, []byte("invalid SQLite contents"), 0600))
	_, err = Open(t.Context(), bad, true, false)
	require.Error(t, err)
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0500))
	defer os.Chmod(dir, 0700)
	_, err = Open(t.Context(), filepath.Join(dir, "unwritable.db"), true, false)
	require.Error(t, err)
}
