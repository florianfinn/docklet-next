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
	_, _, err := createSQLiteArchive(deadline, t.TempDir(), "backup-1", nil, nil, 64)
	require.ErrorIs(t, err, errArchiveTimeout)
	require.NotContains(t, err.Error(), "test-only")
	cancelled, stop := context.WithCancel(t.Context())
	stop()
	require.EqualError(t, archiveContextError(cancelled), "backup.interrupted")
}

func TestReconcileStorageRemovesPartialAndUncommittedArchive(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".partial-old"), []byte("private"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "orphan.tar"), []byte("image"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "complete.tar"), []byte("image"), 0600))
	service := &Service{config: &config.Config{BackupDir: dir}, store: Store{Run: func(_ context.Context, id string) (*backuptypes.SystemBackupRun, error) {
		if id == "complete" {
			return &backuptypes.SystemBackupRun{ID: id, Status: backuptypes.SystemBackupStatusSucceeded, ArchiveName: "complete.tar"}, nil
		}
		return nil, gorm.ErrRecordNotFound
	}}}
	require.NoError(t, service.ReconcileStorage(t.Context()))
	_, err := os.Stat(filepath.Join(dir, ".partial-old"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, "orphan.tar"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, "complete.tar"))
	require.NoError(t, err)
}

func TestReconcileStorageRetainsArchiveOnTransientStoreError(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "keep.tar")
	require.NoError(t, os.WriteFile(name, []byte("image"), 0600))
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
		require.NoError(t, os.WriteFile(filepath.Join(dir, id+".tar"), []byte("image"), 0600))
	}
	var failed []string
	service := &Service{config: &config.Config{BackupDir: dir}, store: Store{
		Run: func(_ context.Context, id string) (*backuptypes.SystemBackupRun, error) {
			return &backuptypes.SystemBackupRun{ID: id, Status: backuptypes.SystemBackupStatusSucceeded, ArchiveName: id + ".tar", Size: 5}, nil
		},
		ListRuns: func(context.Context, pagination.QueryParams) ([]backuptypes.SystemBackupRun, pagination.Response, error) {
			return []backuptypes.SystemBackupRun{
				{ID: "bad", Status: backuptypes.SystemBackupStatusSucceeded, ArchiveName: "bad.tar", Size: 10},
				{ID: "good", Status: backuptypes.SystemBackupStatusSucceeded, ArchiveName: "good.tar", Size: 5},
			}, pagination.Response{}, nil
		},
		FailRun: func(_ context.Context, id, _ string) error { failed = append(failed, id); return nil },
	}}
	require.NoError(t, service.ReconcileStorage(t.Context()))
	require.Equal(t, []string{"bad"}, failed)
	for _, id := range []string{"bad", "good"} {
		_, err := os.Stat(filepath.Join(dir, id+".tar"))
		require.NoError(t, err)
	}
}

func TestReconcileStorageUnavailableIsReportedWithoutPath(t *testing.T) {

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
	for _, name := range []string{"scheduled.tar", "manual.tar"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("image"), 0600))
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
			return &backuptypes.SystemBackupRun{ID: id, PolicyID: "daily", Trigger: trigger, ArchiveName: id + ".tar"}, nil
		},
		DeleteRun: func(_ context.Context, id string) error { deleted = append(deleted, id); return nil },
	}}
	require.NoError(t, service.applyRetention(t.Context(), "daily", 1))
	require.Equal(t, []string{"scheduled"}, deleted)
	_, err := os.Stat(filepath.Join(dir, "scheduled.tar"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, "manual.tar"))
	require.NoError(t, err)
}
