// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original actual archive finalizer proof for inert volume history and preserved target ordering.

package backup

import (
	"context"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
)

func TestArchiveFinalizerDisarmsVolumeObligationsAndPreservesTargetCounter(t *testing.T) {
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
	source := restoreArchiveNew(t)
	source.seed(t)
	store := &backuptarget.Store{DB: source.db}
	targets := backuptarget.NewService(backuptarget.Dependencies{Store: store})
	input := dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "nfs", Server: "nas.example.com", Share: "/archive"}, Name: "First"}
	first, err := targets.Create(t.Context(), input)
	require.NoError(t, err)
	input.Name = "Second"
	input.Share = "/other"
	second, err := targets.Create(t.Context(), input)
	require.NoError(t, err)
	require.NoError(t, source.db.Exec("UPDATE settings SET value='50' WHERE key='backup_target_set_version'").Error)
	require.NoError(t, source.db.Model(&backuptarget.Target{}).Where("id IN ?", []string{first.ID, second.ID}).Update("config_version", 50).Error)
	for _, id := range []string{first.ID, second.ID} {
		require.NoError(t, source.db.Create(&backuptarget.Check{TargetID: id, HostID: "0", SetVersion: 50, ConfigVersion: 50, ActivityID: "old-test", OK: true, At: time.Now().UTC()}).Error)
	}
	oldSet, err := store.SetForHost(t.Context(), "agent")
	require.NoError(t, err)
	oldSet.SetVersion = 100
	agent := restoreArchiveNew(t)
	agentStore := &backuptarget.Store{DB: agent.db, SessionCurrent: func(context.Context, *gorm.DB) error { return nil }}
	_, err = agentStore.Apply(t.Context(), oldSet)
	require.NoError(t, err)
	require.NoError(t, source.db.Table("volume_backups").Omit("type").Create(&volume.Backup{ID: "archived-running", VolumeName: "data", Status: "running", Format: "rustic", Destination: "local", CreatedAt: time.Now().UTC(), StoppedContainers: []volume.StoppedContainer{{ID: "archived-consumer", Name: "app"}}}).Error)
	for _, status := range []string{"running", "failed"} {
		require.NoError(t, source.db.Create(&restore.Run{ID: status, VolumeName: "data", Kind: "full", Mode: "new", NewVolumeName: "restored-data", CreatedVolume: true, CleanupPending: true, PendingRestart: true, Status: status, Phase: "writing", InputPath: "/synthetic/archive/input", StartedAt: time.Now().UTC()}).Error)
	}
	record := restoreArchiveMetadata(t, source)
	target := restoreArchiveInstall(t, source, record)
	require.NoError(t, finalizeRestoredDatabase(t.Context(), target.db, target.cfg))
	var backup volume.Backup
	require.NoError(t, target.db.Table("volume_backups").First(&backup, "id = ?", "archived-running").Error)
	require.Equal(t, "failed", backup.Status)
	require.Equal(t, "volume.backup.interrupted", backup.ErrorCode)
	var restored []restore.Run
	require.NoError(t, target.db.Find(&restored).Error)
	require.Len(t, restored, 2)
	for _, run := range restored {
		require.Equal(t, "failed", run.Status)
		require.False(t, run.PendingRestart)
		require.False(t, run.CleanupPending)
		require.Empty(t, run.InputPath)
		require.True(t, run.CreatedVolume, "historical creation evidence remains inert")
	}
	restoredStore := &backuptarget.Store{DB: target.db}
	authority, version, err := restoredStore.Authority(t.Context())
	require.NoError(t, err)
	require.NotEqual(t, oldSet.Authority, authority)
	require.EqualValues(t, 50, version)
	restoredTargets := backuptarget.NewService(backuptarget.Dependencies{Store: restoredStore})
	input = dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: first.Kind, Server: first.Server, Share: first.Share, Path: first.Path, HostID: first.HostID}, Name: "Edited"}
	edited, err := restoredTargets.Update(t.Context(), first.ID, input)
	require.NoError(t, err)
	require.EqualValues(t, 51, edited.ConfigVersion)
	changed, err := restoredTargets.Get(t.Context(), first.ID)
	require.NoError(t, err)
	require.True(t, changed.Checks[0].EarlierSettings)
	untouched, err := restoredTargets.Get(t.Context(), second.ID)
	require.NoError(t, err)
	require.False(t, untouched.Checks[0].EarlierSettings)
	restoredSet, err := restoredStore.SetForHost(t.Context(), "agent")
	require.NoError(t, err)
	_, err = agentStore.Apply(t.Context(), restoredSet)
	require.NoError(t, err)
}
