// Source: docklet-next/issues/570
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original resolver state, error and role-wiring proofs.; M36 #570: Docklet backup volume and runtime names.

package volume

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBackupVolumeResolver(t *testing.T) {
	for _, tc := range []struct {
		name, explicit, persisted, want string
		inspectErr, writeErr            error
		reads, inspections, writes      int
	}{
		{name: "explicit", explicit: "custom-backups", persisted: "saved-backups", want: "custom-backups", writes: 1},
		{name: "persisted", persisted: "saved-backups", want: "saved-backups", reads: 1, writes: 1},
		{name: "legacy", want: legacyBackupVolumeName, reads: 1, inspections: 1, writes: 1},
		{name: "fresh", want: DefaultBackupVolumeName, inspectErr: errdefs.ErrNotFound, reads: 1, inspections: 1, writes: 1},
		{name: "inspect error", inspectErr: errors.New("offline"), reads: 1, inspections: 1},
		{name: "persist error", writeErr: errors.New("read-only"), reads: 1, inspections: 1, writes: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads, inspections, writes := 0, 0, 0
			r := &BackupVolumeResolver{explicit: tc.explicit,
				read: func(context.Context) (string, error) { reads++; return tc.persisted, nil },
				write: func(_ context.Context, name string) error {
					writes++
					if tc.writeErr == nil {
						require.Equal(t, tc.want, name)
					}
					return tc.writeErr
				},
				inspect: func(_ context.Context, name string) error {
					inspections++
					require.Equal(t, legacyBackupVolumeName, name)
					return tc.inspectErr
				},
			}
			got, err := r.Resolve(t.Context())
			if tc.inspectErr != nil && !errdefs.IsNotFound(tc.inspectErr) || tc.writeErr != nil {
				require.Error(t, err)
				require.Empty(t, got)
				require.Empty(t, r.resolved)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
				again, err := r.Resolve(t.Context())
				require.NoError(t, err)
				require.Equal(t, got, again)
			}
			require.Equal(t, []int{tc.reads, tc.inspections, tc.writes}, []int{reads, inspections, writes})
		})
	}
}

func TestBackupVolumeResolverReadOnlyThenRetry(t *testing.T) {
	inspectErr := errors.New("offline")
	writes := 0
	r := &BackupVolumeResolver{read: func(context.Context) (string, error) { return "", nil }, write: func(context.Context, string) error { writes++; return nil }, inspect: func(context.Context, string) error { return inspectErr }}
	_, err := r.Current(t.Context())
	require.Error(t, err)
	require.Zero(t, writes)
	_, err = r.Resolve(t.Context())
	require.Error(t, err)
	require.Zero(t, writes)
	inspectErr = nil
	name, err := r.Current(t.Context())
	require.NoError(t, err)
	require.Equal(t, legacyBackupVolumeName, name)
	require.Zero(t, writes)
	require.Empty(t, r.resolved)
	name, err = r.Resolve(t.Context())
	require.NoError(t, err)
	require.Equal(t, legacyBackupVolumeName, name)
	require.Equal(t, 1, writes)
}

func TestBackupVolumeResolverEnvRemovalPruneReopen(t *testing.T) {
	ctx := t.Context()
	open := func() *database.DB {
		db, err := gorm.Open(testsqlite.Dialector(t, "resolver-reopen"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&settings.SettingVariable{}))
		return &database.DB{DB: db}
	}
	db := open()
	service := &VolumeService{db: db}
	resolver := service.newBackupVolumeResolver("custom-backups")
	name, err := resolver.Resolve(ctx)
	require.NoError(t, err)
	require.Equal(t, "custom-backups", name)
	svc, err := settings.NewSettingsService(ctx, db)
	require.NoError(t, err)
	require.NoError(t, svc.PruneUnknownSettings(ctx))
	// No override and a new resolver/process must keep the saved identity even after pruning.
	reopened := &VolumeService{db: open()}
	again := reopened.newBackupVolumeResolver("")
	again.inspect = func(context.Context, string) error { t.Fatal("persisted choice must avoid Docker inspect"); return nil }
	name, err = again.Resolve(ctx)
	require.NoError(t, err)
	require.Equal(t, "custom-backups", name)
}

func TestBackupVolumeResolverHubAgentWiring(t *testing.T) {
	for _, role := range []string{"hub", "agent", "edge"} {
		t.Run(role, func(t *testing.T) {
			db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&settings.SettingVariable{}))
			wrapped := &database.DB{DB: db}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/_ping":
					w.Header().Set("API-Version", "1.47")
				case "/v1.41/volumes/arcane-backups":
					_, _ = w.Write([]byte(`{"Name":"arcane-backups"}`))
				case "/v1.41/volumes/docklet-backups":
					_, _ = w.Write([]byte(`{"Name":"docklet-backups"}`))
				default:
					http.Error(w, `{"message":"not found"}`, 404)
				}
			}))
			defer srv.Close()
			ds := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeServiceTestDockerClientInternal(t, srv))
			ss, err := settings.NewSettingsService(t.Context(), wrapped)
			require.NoError(t, err)
			svc := NewVolumeService(wrapped, ds, nil, nil, ss, nil, nil, nil, &config.Config{AgentMode: role == "agent", EdgeAgent: role == "edge"}, nil)
			require.NotNil(t, svc.backupVolume)
			name, err := svc.backupVolume.Current(t.Context())
			require.NoError(t, err)
			require.Equal(t, legacyBackupVolumeName, name)
			var count int64
			require.NoError(t, db.Model(&settings.SettingVariable{}).Where("key = ?", "backupVolumeNameResolved").Count(&count).Error)
			require.Zero(t, count)
			// The child fallback is the actual backup path in both process roles.
			cli, err := ds.GetClient(t.Context())
			require.NoError(t, err)
			storage, err := svc.backup.StorageMount(t.Context(), cli, "/repository", true)
			require.NoError(t, err)
			require.Equal(t, legacyBackupVolumeName, storage.Source)
			var persisted settings.SettingVariable
			require.NoError(t, db.First(&persisted, "key = ?", "backupVolumeNameResolved").Error)
			require.Equal(t, legacyBackupVolumeName, persisted.Value)
		})
	}
}

func TestBackupVolumeResolverRetriesPersistenceFailure(t *testing.T) {
	writeErr := errors.New("temporary write failure")
	writes := 0
	r := &BackupVolumeResolver{explicit: "custom-backups", write: func(context.Context, string) error { writes++; return writeErr }}
	_, err := r.Resolve(t.Context())
	require.Error(t, err)
	require.Empty(t, r.resolved)
	writeErr = nil
	name, err := r.Resolve(t.Context())
	require.NoError(t, err)
	require.Equal(t, "custom-backups", name)
	require.Equal(t, 2, writes)
}

func TestBackupVolumeResolverReadFailure(t *testing.T) {
	r := &BackupVolumeResolver{read: func(context.Context) (string, error) { return "", errors.New("unreadable settings") }, write: func(context.Context, string) error { t.Fatal("read failure must not persist"); return nil }, inspect: func(context.Context, string) error { t.Fatal("read failure must not inspect Docker"); return nil }}
	_, err := r.Resolve(t.Context())
	require.ErrorContains(t, err, "read backup volume selection")
	require.Empty(t, r.resolved)
}
