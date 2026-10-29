// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original host overview history, identity and bounded read-only producer proof.

package backup

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/types/volume"
	mobyvolume "github.com/moby/moby/api/types/volume"
	"github.com/stretchr/testify/require"
)

func TestOverviewRediscoveryMissingVolumesAndHostMetadata(t *testing.T) {
	started := []string{}
	calls := 0
	s := startupService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.True(t, strings.HasSuffix(r.URL.Path, "/volumes"), "overview must only list Docker volumes")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"Volumes": []mobyvolume.Volume{{Name: "live"}, {Name: "hidden", Labels: map[string]string{libarcane.InternalResourceLabel: "true"}}}}))
	}), &started)
	require.NoError(t, s.deps.DB.AutoMigrate(&checkSetting{}, &backuptarget.Target{}, &policies.VolumeBackupPolicy{}))
	require.NoError(t, s.deps.DB.AutoMigrate(activity.Models()...))
	s.deps.Location = time.FixedZone("Owning host", 3600)
	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, s.deps.DB.Exec("INSERT INTO settings(key,value) VALUES ('hubHostId','host-a'),('backupTargetAuthority','authority'),('backupTargetSetVersion','42')").Error)
	require.NoError(t, s.deps.DB.Table("volume_backups").Create(&volume.Backup{ID: "captured", VolumeName: "removed", CreatedAt: now.Add(-time.Hour), Status: "partial", LocalSnapshotID: strings.Repeat("a", 64), Destination: "old-target", Format: "rustic", Size: 7}).Error)
	activity := "capturing"
	require.NoError(t, s.deps.DB.Table("volume_backups").Create(&volume.Backup{ID: "running", VolumeName: "removed", CreatedAt: now, Status: "running", Destination: "new-target", Format: "rustic", ActivityID: &activity}).Error)
	stopped := `[{"id":"exact-id","name":"application"}]`
	require.NoError(t, s.deps.DB.Create(&restore.Run{ID: "restore", VolumeName: "live", Kind: "full", Mode: "replace", Status: "failed", Phase: "writing", PendingRestart: true, StoppedContainers: &stopped, StartedAt: now, ActivityID: "restoring", ErrorCode: "volume.backup.restore-failed"}).Error)
	// A later upload refused before writing must not hide the earlier pending obligation.
	require.NoError(t, s.deps.DB.Create(&restore.Run{ID: "later", VolumeName: "live", Kind: "upload", Mode: "replace", Status: "failed", Phase: "preparing", StartedAt: now.Add(time.Minute), ActivityID: "uploading", ErrorCode: "volume.backup.upload-invalid", ErrorReason: "link"}).Error)
	require.NoError(t, s.deps.DB.Create(&backuptarget.Target{ID: "shared", Name: "Shared", Kind: "nfs", CreatedAt: now}).Error)
	other := "host-b"
	require.NoError(t, s.deps.DB.Create(&backuptarget.Target{ID: "other", Name: "Other", Kind: "local", HostID: &other, CreatedAt: now}).Error)
	result, err := s.Overview(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "Owning host", result.TimeZone)
	require.Equal(t, "authority", result.BackupTargetAuthority)
	require.EqualValues(t, 42, result.BackupTargetSetVersion)
	require.Equal(t, []volume.UsableBackupTarget{{ID: "local", Name: "Local", Kind: "local"}, {ID: "shared", Name: "Shared", Kind: "nfs"}}, result.Targets)
	require.Len(t, result.Volumes, 2)
	live, removed := result.Volumes[0], result.Volumes[1]
	require.Equal(t, "live", live.Name)
	require.True(t, live.VolumeExists)
	require.Equal(t, "later", live.LastRestore.RestoreID)
	require.False(t, live.LastRestore.PendingRestart)
	require.Equal(t, "link", live.LastRestore.Reason)
	require.Len(t, live.PendingRestores, 1)
	require.Equal(t, "restore", live.PendingRestores[0].RestoreID)
	require.True(t, live.PendingRestores[0].PendingRestart)
	require.Equal(t, []volume.StoppedContainer{{ID: "exact-id", Name: "application"}}, live.PendingRestores[0].StoppedContainers)
	require.Empty(t, removed.PendingRestores)
	require.False(t, removed.VolumeExists)
	require.Equal(t, 2, removed.Count)
	require.Equal(t, "running", removed.LastBackup.ID)
	require.Equal(t, &volume.BackupRunning{Kind: "backup", ActivityID: "capturing"}, removed.Running)
	require.True(t, removed.LastSuccessAt.Equal(now.Add(-time.Hour)))
	require.Empty(t, started)
}

// The overview's latest backup is the same entry projection as every other
// backup route; followed operations match it by activity ID.
func TestOverviewLatestBackupMatchesEntryProjection(t *testing.T) {
	started := []string{}
	s := startupService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"Volumes": []mobyvolume.Volume{{Name: "data"}}}))
	}), &started)
	require.NoError(t, s.deps.DB.AutoMigrate(&checkSetting{}, &backuptarget.Target{}, &policies.VolumeBackupPolicy{}))
	require.NoError(t, s.deps.DB.AutoMigrate(activity.Models()...))
	activityID := "11111111-2222-4333-8444-555555555555"
	stored := volume.Backup{
		ID: "latest", VolumeName: "data", CreatedAt: time.Now().UTC().Truncate(time.Second), Status: "failed", Trigger: "manual",
		Destination: "local", Format: volume.BackupFormatRustic, PolicyID: "policy", StopContainers: true,
		ErrorCode: "volume.backup.restart-failed", Error: "volume.backup.restart-failed", ActivityID: &activityID,
	}
	require.NoError(t, s.deps.DB.Table("volume_backups").Create(&stored).Error)
	result, err := s.Overview(t.Context())
	require.NoError(t, err)
	require.Len(t, result.Volumes, 1)
	latest := result.Volumes[0].LastBackup
	require.NotNil(t, latest)
	require.Equal(t, entryInternal(&stored), *latest)
	require.True(t, latest.StopContainers)
	require.Equal(t, &activityID, latest.ActivityID)
	require.Equal(t, "policy", latest.PolicyID)
	require.NotEmpty(t, latest.Type)
	require.Empty(t, started)
}
