// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original coordinated SQLite backup and paired recovery for issue #32.

package backup

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/francis"
	"github.com/florianfinn/docklet-next/backend/resources"
	"github.com/pressly/goose/v3"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
)

type restoreCrashInput struct{ URL, Dir, ID, Point string }

func TestSQLiteRestoreCrashHelper(t *testing.T) {
	input := os.Getenv("DOCKLET_TEST_RESTORE_INPUT")
	if input == "" {
		return
	}
	var args restoreCrashInput
	require.NoError(t, json.Unmarshal([]byte(input), &args))
	cfg := &config.Config{DatabaseURL: args.URL, BackupDir: args.Dir}
	lock, path, err := database.AcquireRestoreLock(t.Context(), args.URL)
	require.NoError(t, err)
	defer lock.Stop()
	require.NoError(t, checkpointStoppedPair(t.Context(), path))
	marker, err := preparePair(t.Context(), cfg, path, args.ID, "")
	require.NoError(t, err)
	if args.Point == "prepared" {
		os.Exit(23)
	}
	stage := restoreStage(path, marker.RestoreID)
	// This exits the process after the first durable rename, rather than merely
	// returning an injected error from the restore function.
	require.NoError(t, copyDurable(t.Context(), filepath.Join(stage, hubImageName), path+".install"))
	require.NoError(t, os.Rename(path+".install", path))
	require.NoError(t, syncArchiveDir(filepath.Dir(path)))
	if args.Point == "hub" {
		os.Exit(23)
	}
	require.NoError(t, installPair(t.Context(), restoreStateDir(path), marker))
	if args.Point == "pair" {
		os.Exit(23)
	}
	db, err := database.OpenLockedForRestore(t.Context(), path, lock)
	require.NoError(t, err)
	require.NoError(t, finalizeRestoredDatabase(t.Context(), db, cfg))
	require.NoError(t, db.SuspendOrdinaryConnections())
	os.Exit(23)
}

func seedSidecar(t *testing.T, path, value string) {
	t.Helper()
	pool, err := sqlitestore.Open(t.Context(), sqlitestore.FrancisPath(path), false, false)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.ExecContext(t.Context(), "INSERT OR REPLACE INTO arcane_francis_actor_state(actor_type,actor_id,actor_state_data) VALUES('example','seed',?)", []byte(value))
	require.NoError(t, err)
	_, err = pool.ExecContext(t.Context(), "INSERT INTO arcane_francis_hosts(host_id,host_address) VALUES('copied-owner','127.0.0.1:3551')")
	require.NoError(t, err)
	_, err = pool.ExecContext(t.Context(), "INSERT INTO arcane_francis_alarms(alarm_id,actor_type,actor_id,alarm_name,alarm_due_time,alarm_lease_id,alarm_lease_expiration_time) VALUES('lease','example','seed','alarm',1,'copied-lease',9999999999999)")
	require.NoError(t, err)
}
func assertRestoredPair(t *testing.T, cfg *config.Config) {
	t.Helper()
	db, err := database.InitializeHub(t.Context(), cfg.DatabaseURL, database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	var value string
	require.NoError(t, db.Raw("SELECT value FROM settings WHERE key='paired-example'").Scan(&value).Error)
	require.Equal(t, "archive", value)
	var active int64
	require.NoError(t, db.Raw("SELECT count(*) FROM user_sessions WHERE revoked_at IS NULL").Scan(&active).Error)
	require.Zero(t, active)
	pool, err := sqlitestore.Open(t.Context(), sqlitestore.FrancisPath(db.Path()), false, false)
	require.NoError(t, err)
	defer pool.Close()
	var state []byte
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT actor_state_data FROM arcane_francis_actor_state WHERE actor_type='example' AND actor_id='seed'").Scan(&state))
	require.Equal(t, "archive", string(state))
	var count int
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT count(*) FROM arcane_francis_hosts").Scan(&count))
	require.Zero(t, count)
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT count(*) FROM arcane_francis_alarms WHERE alarm_lease_id IS NOT NULL").Scan(&count))
	require.Zero(t, count)
}
func TestSQLiteRestoreRecoversProcessInterruption(t *testing.T) {
	for _, point := range []string{"prepared", "hub", "pair", "verified"} {
		t.Run(point, func(t *testing.T) {
			source, cfg := archiveFixture(t)
			require.NoError(t, source.Exec("INSERT INTO settings(key,value) VALUES('paired-example','archive')").Error)
			require.NoError(t, source.Exec("INSERT INTO users(id,created_at,username,password_hash) VALUES('example-user',1,'example','hash')").Error)
			require.NoError(t, source.Exec("INSERT INTO user_sessions(id,created_at,user_id,refresh_token_hash,last_used_at,expires_at) VALUES('example-session',1,'example-user','hash',1,9999999999999999)").Error)
			seedSidecar(t, source.Path(), "archive")
			_, _, err := fixtureArchive(t, source, cfg, "pair", 2<<30)
			require.NoError(t, err)
			target, targetCfg := archiveFixture(t)
			require.NoError(t, target.Exec("INSERT INTO settings(key,value) VALUES('paired-example','safety')").Error)
			seedSidecar(t, target.Path(), "safety")
			targetCfg.BackupDir = cfg.BackupDir
			path := target.Path()
			require.NoError(t, target.Close())
			input, err := json.Marshal(restoreCrashInput{URL: targetCfg.DatabaseURL, Dir: cfg.BackupDir, ID: "pair", Point: point})
			require.NoError(t, err)
			command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestSQLiteRestoreCrashHelper$")
			command.Env = append(os.Environ(), "DOCKLET_TEST_RESTORE_INPUT="+string(input))
			output, err := command.CombinedOutput()
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit, string(output))
			require.Equal(t, 23, exit.ExitCode(), string(output))
			marker, err := readRestoreMarker(restoreStateDir(path))
			require.NoError(t, err)
			recoverCfg := *targetCfg
			recoverCfg.BackupDir = filepath.Join(t.TempDir(), "unavailable-archive-volume")
			require.NoError(t, RecoverPendingRestore(t.Context(), &recoverCfg))
			require.NoError(t, CheckPendingRestore(targetCfg))
			assertRestoredPair(t, targetCfg)
			safety, err := openImage(filepath.Join(restoreStage(path, marker.RestoreID), hubImageName+".safety"))
			require.NoError(t, err)
			defer safety.Close()
			var value string
			require.NoError(t, safety.QueryRow("SELECT value FROM settings WHERE key='paired-example'").Scan(&value))
			require.Equal(t, "safety", value)
		})
	}
}
func TestSQLiteRecoveryRejectsDamagedInputsAndWrongDestination(t *testing.T) {
	source, cfg := archiveFixture(t)
	_, _, err := fixtureArchive(t, source, cfg, "pair", 2<<30)
	require.NoError(t, err)
	target, targetCfg := archiveFixture(t)
	path := target.Path()
	require.NoError(t, target.Close())
	targetCfg.BackupDir = cfg.BackupDir
	marker, err := preparePair(t.Context(), targetCfg, path, "pair", "")
	require.NoError(t, err)
	other := &config.Config{DatabaseURL: sqlitestore.URL(filepath.Join(t.TempDir(), "other.db")), BackupDir: cfg.BackupDir}
	require.NoError(t, writeRestoreMarker(restoreStateDirForTest(t, other), marker))
	require.ErrorIs(t, RecoverPendingRestore(t.Context(), other), ErrRestoreRecoveryRequired)
	image := filepath.Join(restoreStage(path, marker.RestoreID), hubImageName)
	require.NoError(t, os.WriteFile(image, []byte("damaged"), 0600))
	require.ErrorIs(t, RecoverPendingRestore(t.Context(), targetCfg), ErrRestoreRecoveryRequired)
	require.ErrorIs(t, CheckPendingRestore(targetCfg), ErrRestoreRecoveryRequired)
}
func TestSQLiteOfflineRestoreRefusesRunningHub(t *testing.T) {
	_, cfg := archiveFixture(t)
	err := OfflineRestore(t.Context(), cfg, "example")
	require.ErrorIs(t, err, database.ErrHubAlreadyRunning)
	require.NoError(t, CheckPendingRestore(cfg))
}
func TestSQLiteRestoreCancelledBeforeReplacement(t *testing.T) {
	_, cfg := archiveFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.Error(t, OfflineRestore(ctx, cfg, "example"))
	require.NoError(t, CheckPendingRestore(cfg))
	_, err := os.Stat(markerPath(restoreStateDirForTest(t, cfg)))
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func restoreStateDirForTest(t *testing.T, cfg *config.Config) string {
	t.Helper()
	path, err := sqlitestore.Path(cfg.DatabaseURL)
	require.NoError(t, err)
	return restoreStateDir(path)
}

func TestSQLiteRestoreMigratesOlderSupportedPair(t *testing.T) {
	cfg := &config.Config{DatabaseURL: sqlitestore.URL(filepath.Join(t.TempDir(), "source.db")), BackupDir: t.TempDir()}
	source, err := database.OpenHubForRestore(t.Context(), cfg.DatabaseURL)
	require.NoError(t, err)
	defer source.Close()
	pool, err := source.SQLDB()
	require.NoError(t, err)
	migrations, err := fs.Sub(resources.FS, "migrations/sqlite")
	require.NoError(t, err)
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, migrations)
	require.NoError(t, err)
	_, err = provider.UpTo(t.Context(), 1)
	require.NoError(t, err)
	require.NoError(t, francis.InitializeStore(t.Context(), cfg.DatabaseURL))
	require.NoError(t, source.Exec("INSERT INTO settings(key,value) VALUES('older-pair','saved')").Error)
	_, _, err = fixtureArchive(t, source, cfg, "older", 2<<30)
	require.NoError(t, err)
	target := &config.Config{DatabaseURL: sqlitestore.URL(filepath.Join(t.TempDir(), "target.db")), BackupDir: cfg.BackupDir}
	require.NoError(t, OfflineRestore(t.Context(), target, "older"))
	restored, err := database.InitializeHub(t.Context(), target.DatabaseURL, database.MigrationOptions{})
	require.NoError(t, err)
	defer restored.Close()
	var value string
	require.NoError(t, restored.Raw("SELECT value FROM settings WHERE key='older-pair'").Scan(&value).Error)
	require.Equal(t, "saved", value)
	var version int64
	require.NoError(t, restored.Raw("SELECT max(version_id) FROM goose_db_version WHERE is_applied=1").Scan(&version).Error)
	require.Equal(t, mustNewestMigration(), version)
}

func TestSQLitePreparePairRejectsNonemptyWAL(t *testing.T) {
	for _, hub := range []bool{true, false} {
		name := "francis"
		if hub {
			name = "hub"
		}
		t.Run(name, func(t *testing.T) {
			source, cfg := archiveFixture(t)
			_, _, err := fixtureArchive(t, source, cfg, "pair", 2<<30)
			require.NoError(t, err)
			target, targetCfg := archiveFixture(t)
			path := target.Path()
			require.NoError(t, target.Close())
			targetCfg.BackupDir = cfg.BackupDir
			wal := path + "-wal"
			if !hub {
				wal = sqlitestore.FrancisPath(path) + "-wal"
			}
			require.NoError(t, os.WriteFile(wal, []byte("uncheckpointed"), 0600))
			_, err = preparePair(t.Context(), targetCfg, path, "pair", "")
			require.ErrorIs(t, err, ErrRestoreRecoveryRequired)
			_, err = os.Stat(markerPath(restoreStateDir(path)))
			require.True(t, os.IsNotExist(err))
			stages, err := filepath.Glob(path + ".restore-*")
			require.NoError(t, err)
			require.Empty(t, stages)
			data, err := os.ReadFile(wal)
			require.NoError(t, err)
			require.Equal(t, "uncheckpointed", string(data))
		})
	}
}
