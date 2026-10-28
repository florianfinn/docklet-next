// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original fake-Docker startup fence and exact-ID recovery proofs.

package backup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func startupService(t *testing.T, h http.Handler, started *[]string) *Service {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Table("volume_backups").AutoMigrate(&volume.Backup{}))
	require.NoError(t, db.AutoMigrate(&restore.Run{}))
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sql.Close()) })
	wrapped := &database.DB{DB: db}
	return NewService(Dependencies{DB: wrapped, Docker: docker.NewDockerClientService(t.Context(), wrapped, nil, nil).WithClient(cli), Store: Store{Save: func(ctx context.Context, row *volume.Backup) error {
		return db.WithContext(ctx).Table("volume_backups").Save(row).Error
	}}, StartContainer: func(_ context.Context, id string, _ user.Actor) error { *started = append(*started, id); return nil }})
}
func TestStartupUnconfirmedWriterFencesWholeVolumeAndRetries(t *testing.T) {
	confirmed := false
	removed := []string{}
	restarted := []string{}
	inspections := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			// The successful remove reply deliberately does not prove removal.
			require.NoError(t, json.NewEncoder(w).Encode([]container.Summary{{ID: "writer", Labels: map[string]string{rustic.OperationVolumeLabel: "app-data"}}, {ID: "unrelated"}}))
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
	row := &volume.Backup{ID: "backup", VolumeName: "app-data", Status: "running", CreatedAt: time.Now().UTC(), StoppedContainers: []volume.StoppedContainer{{ID: "recorded", Name: "app"}}}
	require.NoError(t, s.deps.DB.Table("volume_backups").Create(row).Error)
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	require.True(t, s.WriterFenced("app-data"))
	require.Empty(t, restarted)
	var stored volume.Backup
	require.NoError(t, s.deps.DB.Table("volume_backups").First(&stored, "id = ?", row.ID).Error)
	require.Equal(t, "running", stored.Status)
	require.ErrorContains(t, s.checkWriterFence(t.Context(), "app-data"), "volume.backup.writer-unconfirmed")
	require.Empty(t, restarted)
	confirmed = true
	require.NoError(t, s.checkWriterFence(t.Context(), "app-data"))
	require.False(t, s.WriterFenced("app-data"))
	require.Equal(t, []string{"recorded"}, restarted)
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	require.Equal(t, []string{"recorded"}, restarted)
	require.NoError(t, s.deps.DB.Table("volume_backups").First(&stored, "id = ?", row.ID).Error)
	require.Equal(t, "failed", stored.Status)
	require.Equal(t, "volume.backup.interrupted", stored.ErrorCode)
	require.Equal(t, 3, inspections)
	require.Equal(t, []string{"writer", "writer", "writer"}, removed)
}
func TestStartupRestartsOnlyRecordedSameIDConsumers(t *testing.T) {
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
	require.NoError(t, s.deps.DB.Table("volume_backups").Create(&volume.Backup{ID: "backup", VolumeName: "app-data", Status: "running", CreatedAt: time.Now().UTC(), StoppedContainers: []volume.StoppedContainer{{ID: "recorded", Name: "app"}, {ID: "removed", Name: "same-name"}}}).Error)
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	require.Equal(t, []string{"recorded"}, restarted)
	// A live operation after startup is neither swept nor settled by the retry.
	require.NoError(t, s.deps.DB.Table("volume_backups").Create(&volume.Backup{ID: "live", VolumeName: "app-data", Status: "running", CreatedAt: time.Now().UTC()}).Error)
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	var live volume.Backup
	require.NoError(t, s.deps.DB.Table("volume_backups").First(&live, "id = ?", "live").Error)
	require.Equal(t, "running", live.Status)
}

func TestStartupAdmissionRemainsClosedUntilSQLAndInputsSettle(t *testing.T) {
	restarted := []string{}
	s := startupService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/volumes") {
			_, _ = w.Write([]byte(`{"Volumes":[]}`))
			return
		}
		t.Errorf("unexpected effect %s", r.URL.Path)
		w.WriteHeader(500)
	}), &restarted)
	entered, release := make(chan struct{}), make(chan struct{})
	s.SetInputCleaner(func(context.Context, []string) error { close(entered); <-release; return nil })
	done := make(chan error, 1)
	go func() { done <- s.ReconcileStartup(t.Context(), false) }()
	<-entered
	require.True(t, s.WriterFenced("any-volume"), "successful Docker discovery is not completed recovery")
	refused := make(chan error, 1)
	go func() { refused <- s.checkWriterFence(t.Context(), "any-volume") }()
	select {
	case err := <-refused:
		require.ErrorContains(t, err, "volume.backup.writer-unconfirmed")
	case <-time.After(time.Second):
		t.Error("admission waited on startup recovery instead of refusing")
	}
	close(release)
	require.NoError(t, <-done)
	require.False(t, s.WriterFenced("any-volume"))
}

func TestStartupSweepsLegacyBackupHelpersAndScratchWithoutTouchingWorkspace(t *testing.T) {
	removed := []string{}
	restarted := []string{}
	scratch := "arcane-rustic-download-11111111-1111-4111-8111-111111111111"
	s := startupService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]container.Summary{
				{ID: "legacy-writer", Image: rustic.DefaultImage, Labels: volumehelper.Labels(), Mounts: []container.MountPoint{{Type: mount.TypeVolume, Name: "app-data", Destination: "/volume"}}},
				{ID: "workspace", Labels: volumehelper.Labels(), Mounts: []container.MountPoint{{Type: mount.TypeVolume, Name: "unrelated-data", Destination: "/volume"}}},
			})
		case strings.HasSuffix(r.URL.Path, "/volumes"):
			_ = json.NewEncoder(w).Encode(map[string]any{"Volumes": []map[string]any{
				{"Name": scratch, "Labels": volumehelper.Labels()},
				{"Name": "unrelated-data", "Labels": volumehelper.Labels()},
				{"Name": "arcane-rustic-download-foreign", "Labels": volumehelper.Labels()},
				{"Name": "foreign-download-11111111-1111-4111-8111-111111111111", "Labels": map[string]string{}},
			}})
		case r.Method == http.MethodDelete:
			removed = append(removed, r.URL.Path)
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/json"):
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}), &restarted)
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	require.Len(t, removed, 2)
	require.Contains(t, removed[0], "legacy-writer")
	require.Contains(t, removed[1], scratch)
	for _, path := range removed {
		require.NotContains(t, path, "unrelated-data")
		require.NotContains(t, path, "workspace")
		require.NotContains(t, path, "foreign")
	}
}
