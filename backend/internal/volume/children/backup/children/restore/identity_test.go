// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original missing-volume and restore-as-new Docker identity proofs.

package restore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullMissingVolumeCreatesWithoutSafetyAndFilesRefuses(t *testing.T) {
	for _, kind := range []string{"full", "files"} {
		t.Run(kind, func(t *testing.T) {
			l, run := lifecycleFixture(t)
			creates := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/volumes/app-data") {
					w.WriteHeader(404)
					_, _ = w.Write([]byte(`{"message":"not found"}`))
					return
				}
				if strings.HasSuffix(r.URL.Path, "/volumes/create") {
					var input client.VolumeCreateOptions
					require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
					require.Equal(t, "app-data", input.Name)
					var stored Run
					require.NoError(t, l.DB.First(&stored, "id = ?", run.ID).Error)
					require.True(t, stored.CreatedVolume, "intent precedes Docker creation")
					creates++
					_, _ = w.Write([]byte(`{"Name":"app-data"}`))
					return
				}
				t.Errorf("unexpected Docker effect %s", r.URL.Path)
				w.WriteHeader(500)
			}))
			defer server.Close()
			cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
			require.NoError(t, err)
			defer cli.Close()
			s := NewService(Dependencies{Docker: docker.NewDockerClientService(t.Context(), l.DB, nil, nil).WithClient(cli),
				Run: func(context.Context, string) (*volume.Backup, error) {
					return &volume.Backup{VolumeName: "app-data", Format: volume.BackupFormatRustic, LocalSnapshotID: strings.Repeat("a", 64)}, nil
				},
				Repository: func(context.Context, *client.Client, *volume.Backup) (backup.Repository, string, error) {
					return backup.Repository{}, strings.Repeat("a", 64), nil
				},
				Password: func(context.Context, *client.Client, ...backup.Repository) (string, error) { return "synthetic", nil }})
			s.SetLifecycle(l)
			request := StartRequest{VolumeName: "app-data", Kind: kind, Mode: "replace"}
			err = s.ValidateAdmission(t.Context(), request)
			if kind == "files" {
				require.ErrorContains(t, err, "volume.not-found")
				require.Zero(t, creates)
				return
			}
			require.NoError(t, err)
			l.Effects.Prepare = func(ctx context.Context, row *Run, r StartRequest) (func() error, error) {
				cleanup, err := s.PrepareRestore(ctx, row, r)
				return cleanup, err
			}
			l.Effects.Safety = func(context.Context, string, user.Actor) (string, error) {
				t.Error("missing target must not get a safety snapshot")
				return "", nil
			}
			l.Effects.Consumers = func(context.Context, string) ([]volume.StoppedContainer, error) { return nil, nil }
			wrote := false
			l.Effects.Write = func(context.Context, *Run, StartRequest) error { wrote = true; return nil }
			require.NoError(t, l.Execute(t.Context(), run, request))
			require.True(t, wrote)
			require.Equal(t, 1, creates)
			require.True(t, run.CreatedVolume)
			require.Empty(t, run.SafetyBackupID)
		})
	}
}

func TestRestoreNewVerifiesCreateIdentityAndRetriesOnlyOwnedCleanup(t *testing.T) {
	for _, scenario := range []string{"owned", "foreign-returned", "lost-create-reply", "cleanup-retry"} {
		t.Run(scenario, func(t *testing.T) {
			l, run := lifecycleFixture(t)
			run.Mode = "new"
			run.NewVolumeName = "recovered-data"
			require.NoError(t, l.DB.Save(run).Error)
			created := false
			removed := 0
			cleanupUnavailable := scenario == "cleanup-retry"
			labels := map[string]string{RestoreLabel: run.ID}
			if scenario == "foreign-returned" {
				labels[RestoreLabel] = "foreign-operation"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/volumes/create"):
					var input client.VolumeCreateOptions
					require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
					require.Equal(t, "recovered-data", input.Name)
					require.Equal(t, run.ID, input.Labels[RestoreLabel])
					var stored Run
					require.NoError(t, l.DB.First(&stored, "id = ?", run.ID).Error)
					require.True(t, stored.CreatedVolume)
					require.Equal(t, "preparing", stored.Phase)
					created = true
					if scenario == "lost-create-reply" {
						w.WriteHeader(503)
						_, _ = w.Write([]byte(`{"message":"reply lost after creation"}`))
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Name": input.Name, "Labels": labels})
				case strings.HasSuffix(r.URL.Path, "/volumes/recovered-data") && r.Method == http.MethodGet:
					if !created {
						w.WriteHeader(404)
						_, _ = w.Write([]byte(`{"message":"not found"}`))
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Name": "recovered-data", "Labels": labels})
				case strings.HasSuffix(r.URL.Path, "/volumes/recovered-data") && r.Method == http.MethodDelete:
					assert.NotEqual(t, "foreign-operation", labels[RestoreLabel], "never remove a foreign same-name volume")
					if cleanupUnavailable {
						w.WriteHeader(503)
						_, _ = w.Write([]byte(`{"message":"cleanup unavailable"}`))
						return
					}
					removed++
					created = false
					w.WriteHeader(204)
				default:
					t.Errorf("unexpected Docker effect %s", r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
			require.NoError(t, err)
			defer cli.Close()
			s := NewService(Dependencies{Docker: docker.NewDockerClientService(t.Context(), l.DB, nil, nil).WithClient(cli),
				Run: func(context.Context, string) (*volume.Backup, error) {
					return &volume.Backup{VolumeName: "app-data", Format: volume.BackupFormatRustic, LocalSnapshotID: strings.Repeat("a", 64)}, nil
				},
				Repository: func(context.Context, *client.Client, *volume.Backup) (backup.Repository, string, error) {
					return backup.Repository{}, strings.Repeat("a", 64), nil
				},
				Password: func(context.Context, *client.Client, ...backup.Repository) (string, error) { return "synthetic", nil }})
			s.SetLifecycle(l)
			l.Effects.Prepare = func(ctx context.Context, row *Run, r StartRequest) (func() error, error) {
				return s.PrepareRestore(ctx, row, r)
			}
			l.Effects.CleanupNew = s.CleanupNew
			wrote := false
			l.Effects.Write = func(context.Context, *Run, StartRequest) error {
				wrote = true
				if scenario == "cleanup-retry" {
					return context.Canceled
				}
				return nil
			}
			err = l.Execute(t.Context(), run, StartRequest{VolumeName: "app-data", Kind: "full", Mode: "new", NewVolumeName: "recovered-data"})
			if scenario == "owned" {
				require.NoError(t, err)
				require.True(t, wrote)
				require.Zero(t, removed)
				return
			}
			require.Error(t, err)
			if scenario == "foreign-returned" {
				require.False(t, wrote)
				require.Zero(t, removed)
				return
			}
			if scenario == "lost-create-reply" {
				require.False(t, wrote)
				require.Equal(t, 1, removed)
				return
			}
			require.True(t, wrote)
			require.True(t, run.CleanupPending)
			require.Zero(t, removed)
			cleanupUnavailable = false
			require.NoError(t, l.Reconcile(t.Context(), false, nil))
			var settled Run
			require.NoError(t, l.DB.First(&settled, "id = ?", run.ID).Error)
			require.False(t, settled.CleanupPending)
			require.Equal(t, 1, removed)
			require.NoError(t, l.Reconcile(t.Context(), false, nil))
			require.Equal(t, 1, removed)
		})
	}
}
