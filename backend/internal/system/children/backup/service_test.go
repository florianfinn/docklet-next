// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original archive engine unit tests for issue #32.

package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestArchiveWriterEnforcesLimitWithoutPartialOverflow(t *testing.T) {
	var out bytes.Buffer
	writer := &limitedArchiveWriter{file: &out, limit: 5}
	n, err := writer.Write([]byte("12345"))
	require.NoError(t, err)
	require.Equal(t, 5, n)
	n, err = writer.Write([]byte("6"))
	require.ErrorIs(t, err, errArchiveLimit)
	require.Zero(t, n)
	require.Equal(t, "12345", out.String())
}

func TestArchiveTimeoutAndCancellation(t *testing.T) {
	deadline, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	require.ErrorIs(t, archiveContextError(deadline), errArchiveTimeout)
	_, _, err := createArchive(deadline, t.TempDir(), "postgres://user:test-only@db.example.com/sample", "backup-1")
	require.ErrorIs(t, err, errArchiveTimeout)
	require.NotContains(t, err.Error(), "test-only")
	cancelled, stop := context.WithCancel(t.Context())
	stop()
	require.EqualError(t, archiveContextError(cancelled), "backup.interrupted")
}

func TestArchiveDSNEnvironment(t *testing.T) {
	env, err := postgresEnv("postgres://user:test-only@db.example.com:5432/sample?sslmode=require")
	require.NoError(t, err)
	require.Contains(t, env, "PGPASSWORD=test-only")
	require.Contains(t, env, "PGSSLMODE=require")
	_, err = postgresEnv("file:sample.db")
	require.ErrorIs(t, err, ErrSystemBackupUnavailable)
}

func TestArchiveProcessLimitsKillDumpAndRemovePartial(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		limit      int64
		timeout    time.Duration
		want       error
	}{
		{name: "size", mode: "limit", limit: 64, timeout: 5 * time.Second, want: errArchiveLimit},
		{name: "deadline", mode: "deadline", limit: 64, timeout: time.Second, want: errArchiveTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			archiveDir := t.TempDir()
			script := `#!/bin/sh
printf '%s\n' "$$" > "$FAKE_DUMP_PID"
printf '%s\n' "$@" > "$FAKE_DUMP_ARGS"
if [ -n "$PGPASSWORD" ]; then printf 'password-set\n' > "$FAKE_DUMP_ENV"; fi
case "$FAKE_DUMP_MODE" in
  limit) while :; do printf 'PGDMP0123456789'; done ;;
  deadline) exec sleep 30 ;;
esac
`
			require.NoError(t, os.WriteFile(filepath.Join(fakeDir, "pg_dump"), []byte(script), 0700))
			pidFile := filepath.Join(fakeDir, "pid")
			argsFile := filepath.Join(fakeDir, "args")
			envFile := filepath.Join(fakeDir, "env")
			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("FAKE_DUMP_PID", pidFile)
			t.Setenv("FAKE_DUMP_ARGS", argsFile)
			t.Setenv("FAKE_DUMP_ENV", envFile)
			t.Setenv("FAKE_DUMP_MODE", tc.mode)
			_, _, err := createArchiveWithLimits(t.Context(), archiveDir,
				"postgres://user:test-only@db.example.com/sample", "bounded", tc.limit, tc.timeout)
			require.ErrorIs(t, err, tc.want)
			require.NotContains(t, err.Error(), "test-only")
			pidBytes, err := os.ReadFile(pidFile)
			require.NoError(t, err)
			pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
			require.NoError(t, err)
			require.ErrorIs(t, syscall.Kill(pid, 0), syscall.ESRCH)
			args, err := os.ReadFile(argsFile)
			require.NoError(t, err)
			require.Contains(t, string(args), "--format=custom")
			require.NotContains(t, string(args), "test-only")
			marker, err := os.ReadFile(envFile)
			require.NoError(t, err)
			require.Equal(t, "password-set\n", string(marker))
			entries, err := os.ReadDir(archiveDir)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestReconcileStorageRemovesPartialAndUncommittedArchive(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".partial-old"), []byte("private"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "orphan.dump"), []byte("PGDMP"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "complete.dump"), []byte("PGDMP"), 0600))
	service := &Service{config: &config.Config{BackupDir: dir}, store: Store{Run: func(_ context.Context, id string) (*backuptypes.SystemBackupRun, error) {
		if id == "complete" {
			return &backuptypes.SystemBackupRun{ID: id, Status: backuptypes.SystemBackupStatusSucceeded, ArchiveName: "complete.dump"}, nil
		}
		return nil, gorm.ErrRecordNotFound
	}}}
	require.NoError(t, service.ReconcileStorage(t.Context()))
	_, err := os.Stat(filepath.Join(dir, ".partial-old"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, "orphan.dump"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, "complete.dump"))
	require.NoError(t, err)
}

func TestReconcileStorageRetainsArchiveOnTransientStoreError(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "keep.dump")
	require.NoError(t, os.WriteFile(name, []byte("PGDMP"), 0600))
	transient := errors.New("temporary store failure")
	service := &Service{config: &config.Config{BackupDir: dir}, store: Store{Run: func(context.Context, string) (*backuptypes.SystemBackupRun, error) {
		return nil, transient
	}}}
	require.ErrorIs(t, service.ReconcileStorage(t.Context()), transient)
	_, err := os.Stat(name)
	require.NoError(t, err)
}

func TestReconcileStorageMarksOnlyInconsistentArchive(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"bad", "good"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, id+".dump"), []byte("PGDMP"), 0600))
	}
	var failed []string
	service := &Service{config: &config.Config{BackupDir: dir}, store: Store{
		Run: func(_ context.Context, id string) (*backuptypes.SystemBackupRun, error) {
			return &backuptypes.SystemBackupRun{ID: id, Status: backuptypes.SystemBackupStatusSucceeded, ArchiveName: id + ".dump", Size: 5}, nil
		},
		ListRuns: func(context.Context, pagination.QueryParams) ([]backuptypes.SystemBackupRun, pagination.Response, error) {
			return []backuptypes.SystemBackupRun{
				{ID: "bad", Status: backuptypes.SystemBackupStatusSucceeded, ArchiveName: "bad.dump", Size: 10},
				{ID: "good", Status: backuptypes.SystemBackupStatusSucceeded, ArchiveName: "good.dump", Size: 5},
			}, pagination.Response{}, nil
		},
		FailRun: func(_ context.Context, id, _ string) error { failed = append(failed, id); return nil },
	}}
	require.NoError(t, service.ReconcileStorage(t.Context()))
	require.Equal(t, []string{"bad"}, failed)
	for _, id := range []string{"bad", "good"} {
		_, err := os.Stat(filepath.Join(dir, id+".dump"))
		require.NoError(t, err)
	}
}

func TestReconcileStorageUnavailableIsReportedWithoutPath(t *testing.T) {
	fakeClientTools(t, "18", "18")
	dir := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(dir, []byte("x"), 0600))
	service := &Service{config: &config.Config{BackupDir: filepath.Join(dir, "backups")}}
	require.ErrorIs(t, service.ReconcileStorage(t.Context()), ErrSystemBackupUnavailable)
	storage := service.Storage()
	require.False(t, storage.Available)
	require.Equal(t, backuptypes.SystemBackupStorageMissing, storage.ReasonCode)
}

func TestOpenArchiveUsesServerSelectedID(t *testing.T) {
	service := &Service{store: Store{Run: func(context.Context, string) (*backuptypes.SystemBackupRun, error) {
		return nil, errors.New("missing")
	}}}
	_, _, _, err := service.OpenArchive(t.Context(), "../outside")
	require.ErrorIs(t, err, ErrSystemBackupNotFound)
}

func TestRetentionKeepsManualArchive(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"scheduled.dump", "manual.dump"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("PGDMP"), 0600))
	}
	deleted := []string{}
	service := &Service{config: &config.Config{BackupDir: dir}, store: Store{
		ExpiredRunIDs: func(context.Context, string, int) ([]string, error) {
			return []string{"scheduled", "manual"}, nil
		},
		Run: func(_ context.Context, id string) (*backuptypes.SystemBackupRun, error) {
			trigger := backuptypes.SystemBackupTriggerScheduled
			if id == "manual" {
				trigger = backuptypes.SystemBackupTriggerManual
			}
			return &backuptypes.SystemBackupRun{ID: id, PolicyID: "daily", Trigger: trigger, ArchiveName: id + ".dump"}, nil
		},
		DeleteRun: func(_ context.Context, id string) error { deleted = append(deleted, id); return nil },
	}}
	require.NoError(t, service.applyRetention(t.Context(), "daily", 1))
	require.Equal(t, []string{"scheduled"}, deleted)
	_, err := os.Stat(filepath.Join(dir, "scheduled.dump"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, "manual.dump"))
	require.NoError(t, err)
}
