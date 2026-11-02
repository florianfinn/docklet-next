// Source: docklet-next/issues/330
// Revision: 20a03b67fbeb6bcc82c758e6f2ece937d58f037a
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original overview running-selection regressions for safety captures, restores and mixed snapshots (M16 reviews CFR8-1, CFR9-1).

package backup

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
)

// A safety capture without a running restore (pre-M12 rows migrated unchanged)
// keeps the volume busy while its unconfirmed writer stays fenced.
func TestOverviewSafetyCaptureWithoutRestoreStaysRunning(t *testing.T) {
	confirmed := false
	restarted := []string{}
	s := startupService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			require.NoError(t, json.NewEncoder(w).Encode([]container.Summary{{
				ID: "legacy-writer", Image: rustic.DefaultImage, Command: "/rustic backup /volume",
				Labels: map[string]string{volumehelper.ContainerLabel: "true"},
				Mounts: []container.MountPoint{{Type: mount.TypeVolume, Name: "data", Destination: "/volume"}},
			}}))
		case strings.HasSuffix(r.URL.Path, "/containers/legacy-writer") && r.Method == http.MethodDelete:
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/containers/legacy-writer/json"):
			if confirmed {
				w.WriteHeader(404)
			} else {
				w.WriteHeader(503)
			}
			_, _ = w.Write([]byte(`{"message":"controlled inspect result"}`))
		case strings.HasSuffix(r.URL.Path, "/volumes"):
			_, _ = w.Write([]byte(`{"Volumes":[{"Name":"data"}]}`))
		default:
			t.Errorf("unexpected Docker effect: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}), &restarted)
	require.NoError(t, s.deps.DB.AutoMigrate(&checkSetting{}, &backuptarget.Target{}, &policies.VolumeBackupPolicy{}))
	require.NoError(t, s.deps.DB.AutoMigrate(activity.Models()...))
	require.NoError(t, s.deps.DB.Table("volume_backups").Create(&volume.Backup{
		ID: "legacy-safety", VolumeName: "data", CreatedAt: time.Now().UTC(), Status: "running",
		Trigger: "safety", Destination: "local", Format: "rustic",
	}).Error)
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	require.True(t, s.WriterFenced("data"))
	_, _, err := s.acquireOperation(t.Context(), "data", "")
	require.ErrorContains(t, err, "volume.backup.writer-unconfirmed")
	held, err := s.HeldForAutoHeal(t.Context(), []string{"data"})
	require.NoError(t, err)
	require.True(t, held)
	var stored volume.Backup
	require.NoError(t, s.deps.DB.Table("volume_backups").First(&stored, "id = ?", "legacy-safety").Error)
	require.Equal(t, "running", stored.Status)
	var parents int64
	require.NoError(t, s.deps.DB.Table("volume_restore_runs").Count(&parents).Error)
	require.Zero(t, parents)
	overview, err := s.Overview(t.Context())
	require.NoError(t, err)
	require.Len(t, overview.Volumes, 1)
	require.Equal(t, &volume.BackupRunning{Kind: "backup"}, overview.Volumes[0].Running,
		"unconfirmed legacy safety capture still owns the volume without a restore row")
	confirmed = true
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	require.False(t, s.WriterFenced("data"))
	overview, err = s.Overview(t.Context())
	require.NoError(t, err)
	require.Nil(t, overview.Volumes[0].Running)
}

func overviewTables(t *testing.T, s *Service) {
	t.Helper()
	require.NoError(t, s.deps.DB.AutoMigrate(&checkSetting{}, &backuptarget.Target{}, &policies.VolumeBackupPolicy{}))
	require.NoError(t, s.deps.DB.AutoMigrate(activity.Models()...))
	require.NoError(t, s.deps.DB.AutoMigrate(&notificationoutbox.Row{}))
}

// Running rows claim the volume newest first; a safety capture claims for its volume's running restore, and without
// one it stays the running backup (class-table rows 1-7, including mixed and equal-time snapshots).
func TestOverviewRunningOrderFollowsClassTable(t *testing.T) {
	cases := []struct {
		name          string
		safety        bool
		restore       bool
		other         string
		otherOffset   time.Duration
		restoreOffset time.Duration
		otherID       string
		want          *volume.BackupRunning
	}{
		{name: "manual-pre-link", other: "backup", otherID: "manual", want: &volume.BackupRunning{Kind: "backup"}},
		{name: "restore-only", restore: true, want: &volume.BackupRunning{Kind: "restore", ActivityID: "restoring", RestoreID: "restore"}},
		{name: "download-only", other: "download", otherID: "download", want: &volume.BackupRunning{Kind: "download", ActivityID: "download"}},
		{name: "safety-only", safety: true, want: &volume.BackupRunning{Kind: "backup"}},
		{name: "paired-safety-newer", safety: true, restore: true, restoreOffset: -2 * time.Second, want: &volume.BackupRunning{Kind: "restore", ActivityID: "restoring", RestoreID: "restore"}},
		{name: "paired-safety-equal-time-higher-id", safety: true, restore: true, want: &volume.BackupRunning{Kind: "restore", ActivityID: "restoring", RestoreID: "restore"}},
		{name: "newest-download-still-wins", safety: true, restore: true, other: "download", otherOffset: time.Second, restoreOffset: -time.Second, otherID: "download", want: &volume.BackupRunning{Kind: "download", ActivityID: "download"}},
		{name: "unpaired-safety-must-not-yield-to-older-download", safety: true, other: "download", otherOffset: -time.Second, otherID: "download", want: &volume.BackupRunning{Kind: "backup"}},
		{name: "unpaired-safety-must-not-yield-to-equal-time-lower-id-download", safety: true, other: "download", otherID: "a-download", want: &volume.BackupRunning{Kind: "backup"}},
		{name: "unpaired-safety-must-not-yield-to-older-manual", safety: true, other: "linked-backup", otherOffset: -time.Second, otherID: "manual", want: &volume.BackupRunning{Kind: "backup"}},
		{name: "paired-safety-equal-time-intervening-download", safety: true, restore: true, other: "download", otherID: "y-download", want: &volume.BackupRunning{Kind: "restore", ActivityID: "restoring", RestoreID: "restore"}},
		{name: "paired-safety-equal-time-intervening-backup", safety: true, restore: true, other: "linked-backup", otherID: "y-backup", want: &volume.BackupRunning{Kind: "restore", ActivityID: "restoring", RestoreID: "restore"}},
		{name: "paired-safety-yields-only-to-restore-not-intervening-download", safety: true, restore: true, restoreOffset: -2 * time.Second, other: "download", otherOffset: -time.Second, otherID: "download", want: &volume.BackupRunning{Kind: "restore", ActivityID: "restoring", RestoreID: "restore"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restarted := []string{}
			s := startupService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"Volumes": []map[string]string{{"Name": "data"}}}))
			}), &restarted)
			overviewTables(t, s)
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			if tc.safety {
				require.NoError(t, s.deps.DB.Table("volume_backups").Create(&volume.Backup{ID: "z-safety", VolumeName: "data", CreatedAt: now, Status: "running", Trigger: "safety", Destination: "local", Format: "rustic"}).Error)
			}
			if tc.restore {
				require.NoError(t, s.deps.DB.Create(&restore.Run{ID: "restore", VolumeName: "data", Kind: "full", Mode: "replace", Status: "running", Phase: "preparing", StartedAt: now.Add(tc.restoreOffset), ActivityID: "restoring"}).Error)
			}
			if tc.other == "download" {
				require.NoError(t, s.deps.DB.Table("activities").Create(map[string]any{"id": tc.otherID, "created_at": now.Add(tc.otherOffset), "environment_id": "0", "type": "resource_action", "status": "running", "resource_type": "volume_backup_download", "resource_id": "data", "started_at": now.Add(tc.otherOffset), "output": "[]"}).Error)
			} else if tc.other != "" {
				row := &volume.Backup{ID: tc.otherID, VolumeName: "data", CreatedAt: now.Add(tc.otherOffset), Status: "running", Trigger: "manual", Destination: "local", Format: "rustic"}
				if tc.other == "linked-backup" {
					id := "capturing"
					row.ActivityID = &id
				}
				require.NoError(t, s.deps.DB.Table("volume_backups").Create(row).Error)
			}
			result, err := s.Overview(t.Context())
			require.NoError(t, err)
			require.Len(t, result.Volumes, 1)
			require.Equal(t, tc.want, result.Volumes[0].Running)
		})
	}
}
