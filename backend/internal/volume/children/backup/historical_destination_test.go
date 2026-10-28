// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original per-consumer historical destination and every-use mount witness proofs.

package backup

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/browser"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

func TestHistoricalConsumersUseRetiredDestinationAndEveryEngineRunWitnessesMount(t *testing.T) {
	for _, operation := range []string{"backup", "restore", "browse", "has-path", "download", "delete-retention", "check"} {
		for _, mounted := range []bool{true, false} {
			t.Run(operation+map[bool]string{true: "-mounted", false: "-unmounted"}[mounted], func(t *testing.T) {
				f := newRuntimePlan(t)
				ctx := t.Context()
				crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
				require.NoError(t, f.service.deps.DB.AutoMigrate(&backuptarget.Target{}, &backuptarget.Check{}, &policies.VolumeBackupPolicy{}))
				target, err := f.service.targets.Create(ctx, dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "mount", HostID: new("0"), Path: "/srv/old-archive"}, Name: "Old archive"})
				require.NoError(t, err)
				retired := time.Now().UTC()
				require.NoError(t, f.service.deps.DB.Model(&backuptarget.Target{}).Where("id = ?", target.ID).Update("retired_at", retired).Error)
				require.NoError(t, f.service.deps.DB.Create(&policies.VolumeBackupPolicy{VolumeName: "data", TargetID: "new-policy-target", LocalEnabled: true}).Error)
				snapshot := strings.Repeat("a", 64)
				row := availabilityRow(t, f, "retained", target.ID, snapshot)
				row.Size = 6
				f.service.deps.Store.Run = func(context.Context, string) (*volume.Backup, error) { return row, nil }
				if !mounted {
					f.witness = "1\n1"
				}
				cli, err := f.service.deps.Docker.GetClient(ctx)
				require.NoError(t, err)
				repo, id, err := f.service.rusticRepositoryForBackupInternal(ctx, cli, row)
				require.NoError(t, err)
				require.Equal(t, snapshot, id)
				b := browser.NewService(browser.Dependencies{Docker: f.service.deps.Docker, Engine: f.service.deps.Engine, Run: f.service.deps.Store.Run, Repository: f.service.rusticRepositoryForBackupInternal, Password: f.service.volumeBackupPasswordInternal, SanitizePath: func(value string) (string, error) { return value, nil }})
				switch operation {
				case "backup":
					f.stdout = `{"id":"` + snapshot + `"}`
					_, err = f.service.deps.Engine.CreateSnapshot(ctx, cli, repo, repo.Password, "fixture", backup.RootSnapshotInput(mount.Mount{Type: mount.TypeVolume, Source: "data", Target: "/source"}))
				case "restore":
					err = f.service.deps.Engine.RestoreSnapshot(ctx, cli, repo, repo.Password, id, mount.Mount{Type: mount.TypeVolume, Source: "data", Target: "/volume"}, backup.RestoreOptions{})
				case "browse":
					_, _, err = b.BrowseBackupFiles(ctx, row.ID, "", pagination.QueryParams{Limit: 20})
				case "has-path":
					_, err = b.BackupHasPath(ctx, row.ID, "empty")
				case "download":
					reader, _, downloadErr := f.service.DownloadBackup(ctx, row.ID, nil)
					err = downloadErr
					if reader != nil {
						require.NoError(t, reader.Close())
					}
				case "delete-retention":
					err = f.service.forgetLocalSnapshotsInternal(ctx, cli, []*volume.Backup{row})
				case "check":
					_, err = f.service.deps.Engine.CheckRepository(ctx, cli, repo, repo.Password, false)
				}
				if !mounted {
					require.ErrorContains(t, err, "backup.target.not-mounted")
				} else {
					require.NoError(t, err)
				}
				engines := 0
				for index, created := range f.creates {
					if created.Image == volumehelper.DefaultToolsImage {
						continue
					}
					engines++
					require.True(t, mounted, "no repository command may run after a failed witness")
					require.Greater(t, index, 0)
					witness := f.creates[index-1]
					require.Equal(t, volumehelper.DefaultToolsImage, witness.Image)
					require.Equal(t, []string{"sleep", "infinity"}, witness.Cmd)
					require.Len(t, witness.HostConfig.Mounts, 2)
					for _, bind := range witness.HostConfig.Mounts {
						require.Contains(t, created.HostConfig.Mounts, bind, "witness and engine must share the identical private bind plan")
						require.Equal(t, mount.PropagationRPrivate, bind.BindOptions.Propagation)
					}
					require.Equal(t, "/srv/old-archive", witness.HostConfig.Mounts[0].Source)
					require.Contains(t, created.Env, "RUSTIC_REPOSITORY=/target/docklet-backups/0/volumes")
				}
				if mounted {
					require.Greater(t, engines, 0)
				} else {
					require.Zero(t, engines)
				}
			})
		}
	}
}
