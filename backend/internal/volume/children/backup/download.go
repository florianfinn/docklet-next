// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Split Rustic download; owned preparation, transport budgets and confirmed cleanup.

package backup

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"
	"uuid"

	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

type edgeDownloadKey struct{}
type ownedDownloadReader struct {
	io.ReadCloser
	cleanup func()
}

func (r *ownedDownloadReader) Close() error { err := r.ReadCloser.Close(); r.cleanup(); return err }

// DownloadBackup owns the volume through packaging, streaming and helper cleanup.
func (s *Service) DownloadBackup(ctx context.Context, id string, actor *user.Actor) (io.ReadCloser, int64, error) {
	entry, err := s.deps.Store.Run(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	if entry.Format != volume.BackupFormatRustic || entry.LocalSnapshotID == "" {
		return nil, 0, dto.NewProblem(404, "volume.backup.snapshot-not-found", nil)
	}
	edge, _ := ctx.Value(edgeDownloadKey{}).(bool)
	if edge && (entry.Size <= 0 || entry.Size > dto.MaxEdgeDownloadBytes) {
		return nil, 0, dto.NewProblem(413, "volume.backup.download-too-large", nil)
	}
	ctx, owner, err := s.acquireOperation(ctx, entry.VolumeName, "")
	if err != nil {
		return nil, 0, err
	}
	budget := 4 * time.Minute
	if edge {
		budget = 3 * time.Minute
	}
	prepare, cancel := context.WithTimeout(ctx, budget)
	reader, size, err := s.downloadRusticBackupInternal(prepare, ctx, entry)
	if err != nil {
		cancel()
		owner.Release(ctx)
		if prepare.Err() == context.DeadlineExceeded {
			return nil, 0, dto.NewProblem(504, "volume.backup.engine-timeout", err)
		}
		return nil, 0, err
	}
	if edge && size > dto.MaxEdgeDownloadBytes {
		_ = reader.Close()
		cancel()
		owner.Release(ctx)
		return nil, 0, dto.NewProblem(413, "volume.backup.download-too-large", nil)
	}
	cleanup := sync.OnceFunc(func() { _ = reader.Close(); cancel(); owner.Release(ctx) })
	context.AfterFunc(ctx, cleanup)
	if actor == nil {
		actor = &user.SystemUser
	}
	if s.deps.Events != nil {
		metadata := database.JSON{"action": "backup_download", "backup_id": id, "size": size}
		if err := s.deps.Events.LogVolumeEvent(ctx, event.EventTypeVolumeBackupDownload, entry.VolumeName, entry.VolumeName, actor.ID, actor.Username, "0", metadata); err != nil {
			slog.WarnContext(ctx, "could not log volume backup download event", "volume", entry.VolumeName, "error", err)
		}
	}
	return &ownedDownloadReader{ReadCloser: reader, cleanup: cleanup}, size, nil
}

func (s *Service) downloadRusticBackupInternal(prepare, stream context.Context, entry *volume.Backup) (io.ReadCloser, int64, error) {
	cli, err := s.deps.Docker.GetClient(prepare)
	if err != nil {
		return nil, 0, err
	}
	repository, snapshot, err := s.rusticRepositoryForBackupInternal(prepare, cli, entry)
	if err != nil {
		return nil, 0, err
	}
	password, err := s.volumeBackupPasswordInternal(prepare, cli, repository)
	if err != nil {
		return nil, 0, err
	}
	scratch := "docklet-backup-download-" + uuid.New().String()
	labels := rustic.OperationLabels(prepare)
	labels[restore.RestoreLabel] = scratch
	if _, err = cli.VolumeCreate(prepare, client.VolumeCreateOptions{Name: scratch, Labels: labels}); err != nil {
		return nil, 0, err
	}
	helper := ""
	cleanup := sync.OnceFunc(func() {
		if helper != "" {
			waitForBackupHelperRemoval(prepare, cli, helper)
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(prepare), 30*time.Second)
		defer cancel()
		_, _ = cli.VolumeRemove(cleanupCtx, scratch, client.VolumeRemoveOptions{})
	})
	failed := true
	defer func() {
		if failed {
			cleanup()
		}
	}()
	scratchMount := mount.Mount{Type: mount.TypeVolume, Source: scratch, Target: "/scratch"}
	image, err := s.deps.HelperImage(prepare, cli)
	if err != nil {
		return nil, 0, err
	}
	created, err := cli.ContainerCreate(prepare, client.ContainerCreateOptions{Config: &container.Config{Image: image, Cmd: []string{"sleep", "infinity"}, NetworkDisabled: true, Labels: rustic.OperationLabels(prepare)}, HostConfig: volumehelper.HostConfig(image, nil, []mount.Mount{scratchMount})})
	if err != nil {
		return nil, 0, err
	}
	helper = created.ID
	if _, err = cli.ContainerStart(prepare, helper, client.ContainerStartOptions{}); err != nil {
		return nil, 0, err
	}
	if _, _, err = s.deps.Exec(prepare, helper, "", []string{"mkdir", "-p", "/scratch/data"}); err != nil {
		return nil, 0, err
	}
	if err = s.deps.Engine.RestoreSnapshot(prepare, cli, repository, password, snapshot, scratchMount, backup.RestoreOptions{DeleteExtra: true, DestinationPath: "/scratch/data"}); err != nil {
		return nil, 0, err
	}
	archive := "/scratch/download.tar.gz"
	if _, _, err = s.deps.Exec(prepare, helper, "", []string{"tar", "-czf", archive, "-C", "/scratch/data", "."}); err != nil {
		return nil, 0, err
	}
	// The preparation budget ends here. CopyFromContainer and the returned stream
	// use the request lifetime, so Direct downloads can stream beyond that budget.
	reader, size, err := volumehelper.DownloadFileFromContainer(stream, cli, helper, archive, cleanup)
	if err != nil {
		return nil, 0, err
	}
	failed = false
	return reader, size, nil
}
