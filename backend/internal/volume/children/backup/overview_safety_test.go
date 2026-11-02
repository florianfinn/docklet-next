// Source: docklet-next/issues/330
// Revision: 20a03b67fbeb6bcc82c758e6f2ece937d58f037a
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original overview regression for a retained safety capture without a restore (M16 review CFR8-1).

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
