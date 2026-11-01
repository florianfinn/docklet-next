// Source: docklet-next/issues/283
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original fake-Docker startup fence and exact-ID recovery proofs; M16 bind mounts and backup completion.

package backup

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBindStartupUnconfirmedWriterFencesWholeKeyAndRetries(t *testing.T) {
	confirmed := false
	removed := []string{}
	restarted := []string{}
	inspections := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			// The successful remove reply deliberately does not prove removal.
			require.NoError(t, json.NewEncoder(w).Encode([]container.Summary{{ID: "writer", Labels: map[string]string{rustic.OperationVolumeLabel: volume.BindKey("/srv/removed")}}, {ID: "unrelated"}}))
		case strings.HasSuffix(r.URL.Path, "/containers/writer") && r.Method == http.MethodDelete:
			removed = append(removed, "writer")
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/containers/writer/json"):
			inspections++
			if confirmed {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"message":"not found"}`))
			} else {
				w.WriteHeader(503)
				_, _ = w.Write([]byte(`{"message":"inspect unavailable"}`))
			}
		case strings.HasSuffix(r.URL.Path, "/containers/recorded/json"):
			_, _ = w.Write([]byte(`{"Id":"recorded","State":{"Running":false}}`))
		case strings.HasSuffix(r.URL.Path, "/volumes"):
			_, _ = w.Write([]byte(`{"Volumes":[]}`))
		default:
			t.Errorf("unexpected Docker effect: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	})
	s := startupService(t, h, &restarted)
	row := &volume.Backup{ID: "backup", VolumeName: volume.BindKey("/srv/removed"), Status: "running", CreatedAt: time.Now().UTC(), StoppedContainers: []volume.StoppedContainer{{ID: "recorded", Name: "app"}}}
	require.NoError(t, s.deps.DB.Table("volume_backups").Create(row).Error)
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	require.True(t, s.WriterFenced(volume.BindKey("/srv/removed")))
	require.Empty(t, restarted)
	var stored volume.Backup
	require.NoError(t, s.deps.DB.Table("volume_backups").First(&stored, "id = ?", row.ID).Error)
	require.Equal(t, "running", stored.Status)
	require.ErrorContains(t, s.checkWriterFence(t.Context(), volume.BindKey("/srv/removed")), "volume.backup.writer-unconfirmed")
	require.Empty(t, restarted)
	confirmed = true
	require.NoError(t, s.checkWriterFence(t.Context(), volume.BindKey("/srv/removed")))
	require.False(t, s.WriterFenced(volume.BindKey("/srv/removed")))
	require.Equal(t, []string{"recorded"}, restarted)
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	require.Equal(t, []string{"recorded"}, restarted)
	require.NoError(t, s.deps.DB.Table("volume_backups").First(&stored, "id = ?", row.ID).Error)
	require.Equal(t, "failed", stored.Status)
	require.Equal(t, "volume.backup.interrupted", stored.ErrorCode)
	require.Equal(t, 3, inspections)
	require.Equal(t, []string{"writer", "writer", "writer"}, removed)
}
func TestBindStartupRestartsOnlyRecordedSameIDConsumers(t *testing.T) {
	restarted := []string{}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			require.NoError(t, json.NewEncoder(w).Encode([]container.Summary{{ID: "unrecorded", Names: []string{"/same-name"}, State: container.StateExited}}))
		case strings.HasSuffix(r.URL.Path, "/volumes"):
			_, _ = w.Write([]byte(`{"Volumes":[]}`))
		case strings.HasSuffix(r.URL.Path, "/containers/recorded/json"):
			_, _ = w.Write([]byte(`{"Id":"recorded","State":{"Running":false}}`))
		case strings.HasSuffix(r.URL.Path, "/containers/removed/json"):
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
		default:
			t.Errorf("unexpected Docker effect %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	s := startupService(t, h, &restarted)
	require.NoError(t, s.deps.DB.Table("volume_backups").Create(&volume.Backup{ID: "backup", VolumeName: volume.BindKey("/srv/removed"), Status: "running", CreatedAt: time.Now().UTC(), StoppedContainers: []volume.StoppedContainer{{ID: "recorded", Name: "app"}, {ID: "removed", Name: "same-name"}}}).Error)
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	require.Equal(t, []string{"recorded"}, restarted)
	// A live operation after startup is neither swept nor settled by the retry.
	require.NoError(t, s.deps.DB.Table("volume_backups").Create(&volume.Backup{ID: "live", VolumeName: volume.BindKey("/srv/removed"), Status: "running", CreatedAt: time.Now().UTC()}).Error)
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	var live volume.Backup
	require.NoError(t, s.deps.DB.Table("volume_backups").First(&live, "id = ?", "live").Error)
	require.Equal(t, "running", live.Status)
}
