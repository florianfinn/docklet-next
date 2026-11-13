// Source: docklet-next/issues/361
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: M21 #438 includes candidate schema; M24 derives the synthetic forward version from the current production chain. Original release startup, refusal, restart and source-boundary proofs.

package database

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/backend/resources"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func testForwardSource(t *testing.T) fs.FS {
	t.Helper()
	source, err := fs.Sub(resources.FS, "migrations/sqlite")
	require.NoError(t, err)
	files := fstest.MapFS{}
	entries, err := fs.ReadDir(source, ".")
	require.NoError(t, err)
	var latest int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, err := strconv.ParseInt(strings.SplitN(entry.Name(), "_", 2)[0], 10, 64)
		require.NoError(t, err)
		latest = max(latest, version)
		data, err := fs.ReadFile(source, entry.Name())
		require.NoError(t, err)
		files[entry.Name()] = &fstest.MapFile{Data: data}
	}
	files[fmt.Sprintf("%03d_fixture.sql", latest+1)] = &fstest.MapFile{Data: []byte("-- +goose Up\nCREATE TABLE future_fixture(value TEXT);\n")}
	return files
}

func TestReleaseFreshStartHistoryIdentityAndSQLSeeds(t *testing.T) {
	for _, hub := range []bool{false, true} {
		name := "agent"
		initialize := Initialize
		if hub {
			name, initialize = "hub", InitializeHub
		}
		t.Run(name, func(t *testing.T) {
			db, err := initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "fresh.db")), MigrationOptions{})
			require.NoError(t, err)
			defer db.Close()
			var versions []int64
			require.NoError(t, db.Raw("SELECT version_id FROM goose_db_version ORDER BY id").Scan(&versions).Error)
			require.Equal(t, []int64{0, 1, 2, 3}, versions)
			var owner string
			require.NoError(t, db.Raw("SELECT identity FROM docklet_database_metadata").Scan(&owner).Error)
			require.Equal(t, "docklet-next:sqlite:2", owner)
			var seeds []struct{ Key, Value string }
			require.NoError(t, db.Raw("SELECT key,value FROM settings ORDER BY key").Scan(&seeds).Error)
			require.Equal(t, []struct{ Key, Value string }{{"updateNotify", "false"}, {"vulnerabilityScanSchedule", `{"mode":"manual"}`}}, seeds)
			require.False(t, db.Migrator().HasTable("environment_update_jobs"))
		})
	}
}

type fileState struct {
	Exists bool
	Hash   [32]byte
	Mode   fs.FileMode
}

func stateOf(t *testing.T, path string) fileState {
	t.Helper()
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fileState{}
	}
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return fileState{true, sha256.Sum256(data), info.Mode().Perm()}
}

func TestPrereleaseHubRefusedBeforeWritableOpen(t *testing.T) {
	for _, journal := range []string{"DELETE", "WAL", "WAL-closed"} {
		t.Run(journal, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "prerelease.db")
			if journal == "WAL" {
				// Exit without SQLite Close/checkpoint, leaving a cold committed WAL input.
				// No SQLite connection remains when preflight reads the source files.
				command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestPrereleaseWALProducer$")
				command.Env = append(os.Environ(), "DOCKLET_TEST_PRERELEASE_WAL="+path)
				output, err := command.CombinedOutput()
				require.NoError(t, err, string(output))
				wal, err := os.Stat(path + "-wal")
				require.NoError(t, err)
				require.Positive(t, wal.Size())
			} else {
				pool := prereleaseFixture(t, path, strings.TrimSuffix(journal, "-closed"))
				require.NoError(t, pool.Close())
			}
			require.NoError(t, os.Chmod(path, 0640))
			francis := sqlitestore.FrancisPath(path)
			require.NoError(t, os.WriteFile(francis, []byte("independent Francis sidecar"), 0640))
			paths := []string{path, path + "-wal", path + "-shm", francis}
			before := make([]fileState, len(paths))
			for i, p := range paths {
				before[i] = stateOf(t, p)
			}
			_, err := InitializeHub(t.Context(), sqlitestore.URL(path), MigrationOptions{})
			require.ErrorContains(t, err, "database.prerelease")
			require.ErrorContains(t, err, path)
			require.ErrorContains(t, err, "new data directory")
			for i, p := range paths {
				require.Equal(t, before[i], stateOf(t, p), filepath.Base(p))
			}
		})
	}
}

func TestInitialMigrationFailureIsRestartable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failed.db")
	failed := fstest.MapFS{"001_failure.sql": &fstest.MapFile{Data: []byte("-- +goose Up\nCREATE TABLE must_rollback(id TEXT);\nINSERT INTO missing_table VALUES(1);\n")}}
	_, err := Initialize(t.Context(), sqlitestore.URL(path), MigrationOptions{Source: failed})
	require.ErrorContains(t, err, "missing_table")
	pool, err := sql.Open("sqlite", sqlitestore.URL(path))
	require.NoError(t, err)
	var tables []string
	rows, err := pool.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	require.NoError(t, err)
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, name)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.NoError(t, pool.Close())
	require.Equal(t, []string{"goose_db_version"}, tables)
	db, err := Initialize(t.Context(), sqlitestore.URL(path), MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	require.True(t, db.Migrator().HasTable("settings"))
	var version int64
	require.NoError(t, db.Raw("SELECT max(version_id) FROM goose_db_version").Scan(&version).Error)
	require.EqualValues(t, 3, version)
}

func TestMigrationSourceSurvivesReopenAndMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "forward.db")
	db, err := Initialize(t.Context(), sqlitestore.URL(path), MigrationOptions{})
	require.NoError(t, err)
	require.NoError(t, db.Close())
	options := MigrationOptions{Source: testForwardSource(t)}
	db, err = InitializeHub(t.Context(), sqlitestore.URL(path), options)
	require.NoError(t, err)
	require.True(t, db.Migrator().HasTable("future_fixture"))
	require.NoError(t, db.SuspendOrdinaryConnections())
	require.NoError(t, db.ResumeOrdinaryConnections())
	require.NoError(t, db.Migrate(t.Context()))
	require.NoError(t, db.Close())
	_, err = Initialize(t.Context(), sqlitestore.URL(path), MigrationOptions{})
	require.ErrorContains(t, err, "newer than supported")
}

func TestRestartableInitialStateRejectsUnrelatedObjects(t *testing.T) {
	for _, statement := range []string{"CREATE TABLE unrelated(id TEXT)", "CREATE VIEW unrelated AS SELECT 1", "CREATE TRIGGER unrelated AFTER INSERT ON goose_db_version BEGIN SELECT 1; END", "UPDATE goose_db_version SET is_applied=0"} {
		path := filepath.Join(t.TempDir(), "initial.db")
		failed := fstest.MapFS{"001_failure.sql": &fstest.MapFile{Data: []byte("-- +goose Up\nINSERT INTO missing_table VALUES(1);\n")}}
		_, err := Initialize(t.Context(), sqlitestore.URL(path), MigrationOptions{Source: failed})
		require.ErrorContains(t, err, "missing_table")
		pool, err := sql.Open("sqlite", sqlitestore.URL(path))
		require.NoError(t, err)
		_, err = pool.Exec(statement)
		require.NoError(t, err)
		require.NoError(t, pool.Close())
		_, err = Initialize(t.Context(), sqlitestore.URL(path), MigrationOptions{})
		require.ErrorContains(t, err, "foreign or inconsistent", statement)
	}
}

func prereleaseFixture(t *testing.T, path, journal string) *sql.DB {
	t.Helper()
	pool, err := sql.Open("sqlite", sqlitestore.URL(path))
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	_, err = pool.Exec("PRAGMA journal_mode=" + journal)
	require.NoError(t, err)
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, os.DirFS("../schema/testdata/prerelease-chain"))
	require.NoError(t, err)
	_, err = provider.Up(t.Context())
	require.NoError(t, err)
	return pool
}

func TestPrereleaseWALProducer(t *testing.T) {
	path := os.Getenv("DOCKLET_TEST_PRERELEASE_WAL")
	if path == "" {
		return
	}
	_ = prereleaseFixture(t, path, "WAL")
	os.Exit(0)
}
