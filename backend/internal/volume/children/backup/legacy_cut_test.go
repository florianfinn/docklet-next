// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original archive read cut, metadata deletion and safety pin regression.

package backup

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/browser"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

func TestLegacyArchiveCannotReadAndDeleteOnlyRemovesMetadata(t *testing.T) {
	effects := 0
	started := []string{}
	s := startupService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { effects++; w.WriteHeader(500) }), &started)
	entry := &volume.Backup{ID: "legacy", VolumeName: "data", Format: "archive", Status: "succeeded", CreatedAt: time.Now().UTC()}
	run := func(context.Context, string) (*volume.Backup, error) { return entry, nil }
	s.deps.Store.Run = run
	s.deps.Engine = backup.NewEngine(t.Context(), newVolumeAdmissionForTestInternal(t), nil)
	_, _, err := s.DownloadBackup(t.Context(), entry.ID, nil)
	var problem *dto.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, "volume.backup.snapshot-not-found", problem.Code)
	reader := browser.NewService(browser.Dependencies{Run: run, Docker: s.deps.Docker, Engine: s.deps.Engine, Repository: func(context.Context, *client.Client, *volume.Backup) (backup.Repository, string, error) {
		return backup.Repository{ID: "local"}, "", nil
	}, Password: func(context.Context, *client.Client, ...backup.Repository) (string, error) { return "synthetic", nil }})
	h := browser.NewHandler(reader)
	_, err = h.ListBackupFiles(t.Context(), &browser.ListBackupFilesInput{BackupID: entry.ID})
	require.ErrorAs(t, err, &problem)
	require.Equal(t, "volume.backup.snapshot-not-found", problem.Code)
	require.Zero(t, effects)
	require.NoError(t, s.deps.DB.AutoMigrate(&event.Event{}))
	s.deps.Events = event.NewEventService(s.deps.DB, nil, nil)
	deleted := []string{}
	s.deps.Store.Delete = func(_ context.Context, id string) error { deleted = append(deleted, id); return nil }
	require.NoError(t, s.DeleteBackup(t.Context(), entry.ID, nil))
	require.Equal(t, []string{entry.ID}, deleted)
	require.Zero(t, effects, "legacy deletion must not open a repository or remove archive files")
	require.NoError(t, s.deps.DB.Create(&restore.Run{ID: "pending", VolumeName: "data", Kind: "full", Mode: "replace", Phase: "writing", Status: "failed", SafetyBackupID: entry.ID, PendingRestart: true, StartedAt: time.Now().UTC()}).Error)
	err = s.DeleteBackup(t.Context(), entry.ID, nil)
	require.ErrorAs(t, err, &problem)
	require.Equal(t, "volume.backup.safety-pinned", problem.Code)
	require.Equal(t, []string{entry.ID}, deleted)
}
