// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original historical destination consumers through registered H/D/E routes.

package backup

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

func TestHistoricalDestinationConsumersThroughHubDirectAndActualEdge(t *testing.T) {
	for _, mode := range []string{"hub", "direct", "edge"} {
		for _, operation := range []string{"restore", "browse", "download", "delete", "retention"} {
			t.Run(mode+"/"+operation, func(t *testing.T) {
				f := newRuntimePlan(t)
				f.volumeState = map[string]map[string]string{}
				db := f.service.deps.DB
				require.NoError(t, db.AutoMigrate(&backuptarget.Target{}, &backuptarget.Check{}, &policies.VolumeBackupPolicy{}, &event.Event{}))
				require.NoError(t, db.AutoMigrate(activity.Models()...))
				crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
				host := "0"
				if mode != "hub" {
					host = "owning-host"
					require.NoError(t, db.Exec("INSERT INTO settings(key,value) VALUES ('hubHostId',?)", host).Error)
				}
				target, err := f.service.targets.Create(t.Context(), dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "mount", HostID: &host, Path: "/srv/old-archive"}, Name: "Old archive"})
				require.NoError(t, err)
				require.NoError(t, db.Model(&backuptarget.Target{}).Where("id = ?", target.ID).Update("retired_at", time.Now().UTC()).Error)
				require.NoError(t, db.Create(&policies.VolumeBackupPolicy{VolumeName: "data", TargetID: "new-target", LocalEnabled: true}).Error)
				row := availabilityRow(t, f, "row", target.ID, strings.Repeat("a", 64))
				require.NoError(t, db.Table("volume_backups").Where("id = ?", row.ID).Updates(map[string]any{"size": 6, "trigger": "scheduled"}).Error)
				f.service.deps.Activity = activity.NewActivityService(db, nil)
				f.service.deps.Events = event.NewEventService(db, nil, nil)
				f.service.deps.Store.Run = func(ctx context.Context, id string) (*volume.Backup, error) {
					var row volume.Backup
					err := db.WithContext(ctx).Table("volume_backups").First(&row, "id = ?", id).Error
					return &row, err
				}
				f.service.deps.Store.Delete = func(ctx context.Context, id string) error {
					return db.WithContext(ctx).Table("volume_backups").Where("id = ?", id).Delete(&volume.Backup{}).Error
				}
				f.service = NewService(f.service.deps)
				router, rawAPI := humatest.New(t)
				api := huma.NewGroup(rawAPI, "/api")
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					request := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
					request = context.WithValue(request, user.CurrentUserContextKey{}, &user.SystemUser)
					next(huma.WithContext(ctx, request))
				})
				cfg := &config.Config{AgentMode: mode == "direct", EdgeAgent: mode == "edge"}
				RegisterRoutes(api, NewHandler(f.service, f.service.deps.Activity, nil, nil, t.Context(), cfg))
				method, path := "GET", "/api/environments/0/volumes/backups/row/files/browse"
				var payload []byte
				switch operation {
				case "restore":
					method = "POST"
					path = "/api/environments/0/volumes/data/backups/row/restore"
					payload = []byte(`{}`)
				case "download":
					path = "/api/environments/0/volumes/backups/row/download"
				case "delete":
					method = "DELETE"
					path = "/api/environments/0/volumes/backups/row"
				case "retention":
					require.NoError(t, db.Table("volume_backups").Create(&volume.Backup{ID: "newest", VolumeName: "data", CreatedAt: time.Now().UTC().Add(time.Hour), Trigger: "scheduled", Status: "succeeded", Format: "rustic", Destination: "new-target", LocalSnapshotID: strings.Repeat("b", 64)}).Error)
					owned, owner, err := f.service.acquireOperation(t.Context(), "data", "")
					require.NoError(t, err)
					require.NoError(t, f.service.applyVolumeBackupRetentionInternal(owned, "data", "newest", 1))
					owner.Release(owned)
				}
				if operation != "retention" {
					status, _, body := backupRouteTransport(t, mode, router)(t.Context(), method, path, payload)
					if operation == "restore" {
						require.Equal(t, 202, status, string(body))
						require.Eventually(t, func() bool { var row restore.Run; return db.First(&row).Error == nil && row.Status == "succeeded" }, 3*time.Second, 5*time.Millisecond)
						require.NoError(t, f.service.deps.Activity.CancelAndDrain(t.Context(), ""))
					} else {
						require.Equal(t, 200, status, string(body))
					}
				}
				engines := 0
				for index, created := range f.creates {
					if created.Image == volumehelper.DefaultToolsImage {
						continue
					}
					engines++
					require.Greater(t, index, 0)
					require.Equal(t, "/srv/old-archive", f.creates[index-1].HostConfig.Mounts[0].Source)
					require.Contains(t, created.Env, "RUSTIC_REPOSITORY=/target/docklet-backups/"+host+"/volumes")
					require.NotContains(t, strings.Join(created.Env, " "), "new-target")
				}
				require.Positive(t, engines)
				if operation == "delete" || operation == "retention" {
					var count int64
					require.NoError(t, db.Table("volume_backups").Where("id = ?", "row").Count(&count).Error)
					require.Zero(t, count)
				}
			})
		}
	}
}
