// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original upload ownership, expiry separation and compressed-byte bounds.

package upload

import (
	"bytes"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/upload"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"
)

func TestRestoreInputMovesOutOfSessionExpiry(t *testing.T) {
	s := &UploadService{root: t.TempDir()}
	payload := []byte("owned bytes")
	session, err := s.IngestSession(t.Context(), upload.KindVolumeBackup, "backup.tgz", int64(len(payload)), bytes.NewReader(payload))
	require.NoError(t, err)
	before, err := os.Stat(filepath.Join(s.root, session.ID, "data"))
	require.NoError(t, err)
	id := uuid.New().String()
	owned, err := s.TakeRestoreInput(t.Context(), session.ID, id)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(s.root, "restore-inputs", id, "input"), owned)
	after, err := os.Stat(owned)
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after), "ownership must rename on the same filesystem")
	_, err = s.GetSession(t.Context(), upload.KindVolumeBackup, session.ID)
	require.Error(t, err)
	_, err = s.TakeRestoreInput(t.Context(), session.ID, uuid.New().String())
	require.Error(t, err)
	removed, err := s.PurgeExpiredSessions(t.Context(), -time.Hour)
	require.NoError(t, err)
	require.Zero(t, removed)
	got, err := os.ReadFile(owned)
	require.NoError(t, err)
	require.Equal(t, payload, got)
}
func TestVolumeUploadCapAtCreationAndChunk(t *testing.T) {
	s := &UploadService{root: t.TempDir()}
	_, err := s.CreateSession(t.Context(), upload.KindVolumeBackup, upload.CreateSessionRequest{Filename: "backup.tgz", Size: dto.MaxVolumeUploadBytes + 1})
	require.Error(t, err)
	var problem *dto.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, 413, problem.Status)
	session, err := s.CreateSession(t.Context(), upload.KindVolumeBackup, upload.CreateSessionRequest{Filename: "backup.tgz", Size: 1})
	require.NoError(t, err)
	session.Size = dto.MaxVolumeUploadBytes + 1
	require.NoError(t, s.writeMetaInternal(t.Context(), session))
	_, err = s.WriteChunk(t.Context(), upload.KindVolumeBackup, session.ID, 0, []byte("x"))
	require.ErrorAs(t, err, &problem)
	require.Equal(t, "volume.backup.upload-too-large", problem.Code)
}

func TestStartupRestoreInputCleanupRetainsRunningAndNeverSweepsNewAdmissions(t *testing.T) {
	s := &UploadService{root: t.TempDir()}
	root := filepath.Join(s.root, "restore-inputs")
	makeInput := func(id string) string {
		file := filepath.Join(root, id, "input")
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0700))
		require.NoError(t, os.WriteFile(file, []byte("example"), 0600))
		return file
	}
	orphan := makeInput("orphan")
	terminal := makeInput("terminal")
	running := makeInput("running")
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("outside"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "link")))
	require.NoError(t, s.CleanupRestoreInputs(t.Context(), []string{"running"}))
	require.NoFileExists(t, orphan)
	require.NoFileExists(t, terminal)
	require.FileExists(t, running)
	require.FileExists(t, sentinel)
	// This transfer is intentionally not in a DB row yet: startup retries may
	// touch only directories observed before admissions opened.
	live := makeInput("new-live-admission")
	require.NoError(t, s.CleanupRestoreInputs(t.Context(), nil))
	require.NoFileExists(t, running)
	require.FileExists(t, live)
	require.FileExists(t, sentinel)
	require.NoError(t, s.CleanupRestoreInputs(t.Context(), nil))
	require.FileExists(t, live)
}
