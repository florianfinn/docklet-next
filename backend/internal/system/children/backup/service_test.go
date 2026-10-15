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
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
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

func TestArchiveDSNEnvironmentDoesNotExposePasswordInArguments(t *testing.T) {
	env, err := postgresEnv("postgres://user:test-only@db.example.com:5432/sample?sslmode=require")
	require.NoError(t, err)
	require.Contains(t, env, "PGPASSWORD=test-only")
	require.Contains(t, env, "PGSSLMODE=require")
	require.NotContains(t, strings.Join([]string{"--format=custom", "--no-owner"}, " "), "test-only")
	_, err = postgresEnv("file:sample.db")
	require.ErrorIs(t, err, ErrSystemBackupUnavailable)
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
		return nil, errors.New("missing")
	}}}
	require.NoError(t, service.ReconcileStorage(t.Context()))
	_, err := os.Stat(filepath.Join(dir, ".partial-old"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, "orphan.dump"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, "complete.dump"))
	require.NoError(t, err)
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
