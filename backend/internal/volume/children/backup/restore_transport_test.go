// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original full and upload restore admission and Docker identity through H/D/E.

package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/upload"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/base"
	uploadtypes "github.com/florianfinn/docklet-next/types/upload"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
)

func portableRestoreInput(t *testing.T, uploads *upload.UploadService) string {
	t.Helper()
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "file.txt", Mode: 0600, Size: 7}))
	_, err := tw.Write([]byte("payload"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	session, err := uploads.CreateSession(t.Context(), uploadtypes.KindVolumeBackup, uploadtypes.CreateSessionRequest{Filename: "backup.tar.gz", Size: int64(data.Len())})
	require.NoError(t, err)
	_, err = uploads.WriteChunk(t.Context(), uploadtypes.KindVolumeBackup, session.ID, 0, data.Bytes())
	require.NoError(t, err)
	return session.ID
}

func TestRestoreMissingAndNewVolumeThroughHubDirectAndActualEdge(t *testing.T) {
	for _, mode := range []string{"hub", "direct", "edge"} {
		for _, kind := range []string{"full", "upload", "files"} {
			for _, scenario := range []string{"missing", "new", "foreign"} {
				if kind == "files" && scenario != "missing" {
					continue
				}
				t.Run(mode+"/"+kind+"/"+scenario, func(t *testing.T) {
					t.Setenv("TMPDIR", t.TempDir())
					f := newRuntimePlan(t)
					f.volumeState = map[string]map[string]string{}
					f.foreignCreate = scenario == "foreign"
					db := f.service.deps.DB
					require.NoError(t, db.AutoMigrate(activity.Models()...))
					require.NoError(t, db.AutoMigrate(&policies.VolumeBackupPolicy{}))
					f.service.deps.Activity = activity.NewActivityService(db, nil)
					f.service.deps.Store.Run = func(context.Context, string) (*volume.Backup, error) {
						return &volume.Backup{ID: "row", VolumeName: "data", Format: volume.BackupFormatRustic, LocalSnapshotID: strings.Repeat("a", 64), Destination: "local"}, nil
					}
					f.service = NewService(f.service.deps)
					entered, release := make(chan struct{}), make(chan struct{})
					actualWrite := f.service.lifecycle.Effects.Write
					f.service.lifecycle.Effects.Write = func(ctx context.Context, run *restore.Run, r restore.StartRequest) error {
						close(entered)
						<-release
						return actualWrite(ctx, run, r)
					}
					var once bool
					defer func() {
						if !once {
							close(release)
						}
					}()
					uploads := upload.NewUploadService(nil)
					router, rawAPI := humatest.New(t)
					api := huma.NewGroup(rawAPI, "/api")
					api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
						request := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
						request = context.WithValue(request, user.CurrentUserContextKey{}, &user.SystemUser)
						next(huma.WithContext(ctx, request))
					})
					cfg := &config.Config{AgentMode: mode == "direct", EdgeAgent: mode == "edge"}
					RegisterRoutes(api, NewHandler(f.service, f.service.deps.Activity, nil, uploads, t.Context(), cfg))
					request := map[string]any{}
					path := "/api/environments/0/volumes/data/backups/row/restore"
					if scenario != "missing" {
						request["mode"] = "new"
						request["newVolumeName"] = "recovered-data"
					}
					if kind == "upload" {
						path = "/api/environments/0/volumes/data/backups/upload"
						request["uploadId"] = portableRestoreInput(t, uploads)
					}
					if kind == "files" {
						path += "-files"
						request = map[string]any{"paths": []string{"file.txt"}}
					}
					payload, err := json.Marshal(request)
					require.NoError(t, err)
					status, _, body := backupRouteTransport(t, mode, router)(t.Context(), "POST", path, payload)
					if kind == "files" {
						require.Equal(t, 404, status, string(body))
						require.Contains(t, string(body), "volume.not-found")
						require.Empty(t, f.volumeState)
						return
					}
					require.Equal(t, 202, status, string(body))
					var accepted base.ApiResponse[volume.RestoreOperation]
					require.NoError(t, json.Unmarshal(body, &accepted))
					require.NotEmpty(t, accepted.Data.ActivityID)
					require.NotEmpty(t, accepted.Data.RestoreID)
					if scenario != "foreign" {
						select {
						case <-entered:
						case <-time.After(2 * time.Second):
							t.Fatal("accepted writer never started")
						}
						var row restore.Run
						require.NoError(t, db.First(&row, "id = ?", accepted.Data.RestoreID).Error)
						require.Equal(t, "writing", row.Phase)
						require.True(t, row.CreatedVolume)
						require.Empty(t, row.SafetyBackupID, "missing and new destinations skip safety")
						for _, name := range []string{"data", row.NewVolumeName} {
							if name == "" {
								continue
							}
							unlock, ok := f.service.deps.Locks.TryLock(name)
							if ok {
								unlock()
							}
							require.False(t, ok, "accepted restore owns both names before writing")
						}
						if scenario == "new" {
							require.Equal(t, row.ID, f.volumeState["recovered-data"][restore.RestoreLabel])
							expected := "row"
							if kind == "upload" {
								expected = ""
							}
							require.Equal(t, expected, f.volumeState["recovered-data"]["com.getarcaneapp.restored-from"])
						}
						if kind == "upload" {
							_, err := os.Stat(row.InputPath)
							require.NoError(t, err, "202 must retain operation-owned upload")
						}
					}
					close(release)
					once = true
					require.Eventually(t, func() bool {
						var row restore.Run
						if db.First(&row, "id = ?", accepted.Data.RestoreID).Error != nil {
							return false
						}
						return row.Status != "running"
					}, 3*time.Second, 5*time.Millisecond)
					require.NoError(t, f.service.deps.Activity.CancelAndDrain(t.Context(), ""))
					var row restore.Run
					require.NoError(t, db.First(&row, "id = ?", accepted.Data.RestoreID).Error)
					if scenario == "foreign" {
						require.Equal(t, "failed", row.Status)
						require.Equal(t, "volume.exists", row.ErrorCode)
						require.Equal(t, "foreign-operation", f.volumeState["recovered-data"][restore.RestoreLabel])
					} else {
						require.Equal(t, "succeeded", row.Status)
					}
					if kind == "upload" {
						_, err := os.Stat(row.InputPath)
						require.True(t, os.IsNotExist(err), "terminal worker removes its input")
					}
					unlock, ok := f.service.deps.Locks.TryLock("data")
					require.True(t, ok)
					unlock()
				})
			}
		}
	}
}
