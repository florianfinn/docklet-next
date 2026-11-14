// Source: docklet-next/issues/386
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original two-phase hub-local and Direct snapshot download for issue #386.

package backup

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker"
)

// StreamingDownload is an admitted hub-local or Direct snapshot download. Admission refusals are typed problems
// from BeginStreamingDownload; once Stream has written the first byte, failures can only abort the response.
type StreamingDownload struct {
	s          *Service
	entry      *volume.Backup
	ctx        context.Context
	prepare    context.Context
	cancel     context.CancelFunc
	cli        *client.Client
	repository backup.Repository
	snapshot   string
	password   string
	release    func(error)
}

// BeginStreamingDownload admits a download synchronously: snapshot identity, volume operation, activity,
// repository and password. Stream must follow; it releases the admission.
func (s *Service) BeginStreamingDownload(ctx context.Context, id string, actor *user.Actor) (*StreamingDownload, error) {
	entry, err := s.deps.Store.Run(ctx, id)
	if err != nil {
		return nil, err
	}
	if entry.Format != volume.BackupFormatRustic || entry.LocalSnapshotID == "" {
		return nil, dto.NewProblem(404, "volume.backup.snapshot-not-found", nil)
	}
	ctx, owner, err := s.acquireOperation(ctx, entry.VolumeName, "")
	if err != nil {
		return nil, err
	}
	if actor == nil {
		actor = &user.SystemUser
	}
	activityID, workCtx := activitylib.StartHandlerActivity(ctx, s.deps.Activity, "0", activitytypes.TypeResourceAction, "volume_backup_download", entry.VolumeName, volume.DisplayName(entry.VolumeName), actor, "Preparing backup download", "Preparing backup download", database.JSON{"action": "download_volume_backup", "backupId": entry.ID}, false)
	if s.deps.Activity != nil && activityID == "" {
		owner.Release(ctx)
		return nil, dto.NewProblem(503, "volume.backup.engine-unavailable", nil)
	}
	// The preparation budget covers repository resolution and restore; the archive transfer uses the request lifetime.
	budget := s.deps.DownloadPrepareBudget
	if budget <= 0 {
		budget = 4 * time.Minute
	}
	prepare, cancel := context.WithTimeout(workCtx, budget)
	d := &StreamingDownload{s: s, entry: entry, ctx: workCtx, prepare: prepare, cancel: cancel}
	var once sync.Once
	d.release = func(err error) {
		once.Do(func() {
			cancel()
			owner.Release(workCtx)
			if err == nil {
				err = workCtx.Err()
			}
			activitylib.CompleteHandlerActivity(workCtx, s.deps.Activity, activityID, "Backup downloaded", neutralBackupError(err, "volume.backup.repository-unavailable"))
		})
	}
	d.cli, err = s.deps.Docker.GetClient(prepare)
	if err == nil {
		d.repository, d.snapshot, err = s.rusticRepositoryForBackupInternal(prepare, d.cli, entry)
	}
	if err == nil {
		d.password, err = s.volumeBackupPasswordInternal(prepare, d.cli, d.repository)
	}
	if err != nil {
		err = d.timeoutProblem(err)
		d.release(err)
		return nil, err
	}
	if s.deps.Events != nil {
		metadata := database.JSON{"action": "backup_download", "backup_id": id, "size": entry.Size}
		if err := s.deps.Events.LogVolumeEvent(ctx, event.EventTypeVolumeBackupDownload, entry.VolumeName, volume.DisplayName(entry.VolumeName), actor.ID, actor.Username, "0", metadata); err != nil {
			slog.WarnContext(ctx, "could not log volume backup download event", "volume", entry.VolumeName, "error", err)
		}
	}
	return d, nil
}

func (d *StreamingDownload) timeoutProblem(err error) error {
	if d.prepare.Err() == context.DeadlineExceeded {
		return dto.NewProblem(504, "volume.backup.engine-timeout", err)
	}
	return err
}

// Stream writes one gzip member: the header at once, an empty sync block every keep-alive interval while the
// snapshot restores, then the tar of the restored tree. All writes happen on the caller's goroutine except the
// tar copy, which starts after the last keep-alive. The helper is removed before the admission is released.
func (d *StreamingDownload) Stream(w io.Writer, flush func()) (err error) {
	scratch := "docklet-backup-download-" + uuid.New().String()
	helper := ""
	created := false
	defer func() {
		if helper != "" {
			waitForBackupHelperRemoval(d.prepare, d.cli, helper)
		}
		if created {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(d.ctx), 30*time.Second)
			_, _ = d.cli.VolumeRemove(cleanupCtx, scratch, client.VolumeRemoveOptions{})
			cancel()
		}
		d.release(err)
	}()
	archive := gzip.NewWriter(w)
	if err = archive.Flush(); err != nil {
		return err
	}
	flush()
	ready := make(chan error, 1)
	var helperID string
	go func() {
		id, prepareErr := d.restoreInternal(scratch, &created)
		helperID = id
		ready <- prepareErr
	}()
	interval := d.s.deps.DownloadKeepAlive
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for waiting := true; waiting; {
		select {
		case prepareErr := <-ready:
			helper, waiting = helperID, false
			if prepareErr != nil {
				return d.timeoutProblem(prepareErr)
			}
		case <-ticker.C:
			if writeErr := archive.Flush(); writeErr != nil {
				d.cancel()
				prepareErr := <-ready
				helper = helperID
				return errors.Join(writeErr, prepareErr)
			}
			flush()
		}
	}
	ticker.Stop()
	if err = d.execStream(helper, []string{"tar", "-cf", "-", "-C", "/scratch/data", "."}, archive); err != nil {
		return err
	}
	if err = archive.Close(); err != nil {
		return err
	}
	flush()
	return d.ctx.Err()
}

// execStream passes the helper's stdout through unbuffered and keeps at most 64 KiB of stderr for the error.
// The attach stream can end before the process does, so success needs a terminated exec with exit 0 and a
// stream that ends with the tar end-of-archive blocks.
func (d *StreamingDownload) execStream(helper string, cmd []string, stdout io.Writer) error {
	if d.s.deps.ExecStream != nil {
		return d.s.deps.ExecStream(d.ctx, helper, cmd, stdout)
	}
	created, err := d.cli.ExecCreate(d.ctx, helper, client.ExecCreateOptions{AttachStdout: true, AttachStderr: true, Cmd: cmd})
	if err != nil {
		return err
	}
	attach, err := d.cli.ExecAttach(d.ctx, created.ID, client.ExecAttachOptions{})
	if err != nil {
		return err
	}
	defer attach.Close()
	stop := context.AfterFunc(d.ctx, attach.Close)
	defer stop()
	stderr := &boundedBuffer{limit: 64 << 10}
	tail := &archiveTail{}
	if err := docker.WaitStdCopy(docker.StartStdCopy(attach.Reader, io.MultiWriter(stdout, tail), stderr)); err != nil {
		return err
	}
	if err := d.ctx.Err(); err != nil {
		return err
	}
	exitCode, err := d.awaitExecExit(created.ID)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("%s exited with status %d: %s", cmd[0], exitCode, strings.TrimSpace(stderr.String()))
	}
	if !tail.complete() {
		return errors.New("archive stream ended before the tar end-of-archive blocks")
	}
	return nil
}

// awaitExecExit waits briefly for the exec to terminate after its output ended; a process still running then
// means the output stream was cut short.
func (d *StreamingDownload) awaitExecExit(id string) (int, error) {
	deadline := time.Now().Add(execExitWait)
	for {
		inspect, err := d.cli.ExecInspect(d.ctx, id, client.ExecInspectOptions{})
		if err != nil {
			return 0, err
		}
		if !inspect.Running {
			return inspect.ExitCode, nil
		}
		if time.Now().After(deadline) {
			return 0, errors.New("archive exec still running after its output ended")
		}
		select {
		case <-d.ctx.Done():
			return 0, d.ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

var execExitWait = 5 * time.Second

// archiveTail tracks the stream length and its last 1024 bytes: a complete tar stream is a whole number of
// 512-byte blocks ending with two zero blocks.
type archiveTail struct {
	size int64
	last [1024]byte
}

func (t *archiveTail) Write(p []byte) (int, error) {
	t.size += int64(len(p))
	if len(p) >= len(t.last) {
		copy(t.last[:], p[len(p)-len(t.last):])
	} else {
		copy(t.last[:], t.last[len(p):])
		copy(t.last[len(t.last)-len(p):], p)
	}
	return len(p), nil
}

func (t *archiveTail) complete() bool {
	return t.size >= int64(len(t.last)) && t.size%512 == 0 && t.last == [1024]byte{}
}

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.Len(); room > 0 {
		b.Buffer.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// restoreInternal creates the scratch volume and helper and restores the snapshot into /scratch/data.
func (d *StreamingDownload) restoreInternal(scratch string, created *bool) (string, error) {
	labels := rustic.OperationLabels(d.prepare)
	labels[restore.RestoreLabel] = scratch
	if _, err := d.cli.VolumeCreate(d.prepare, client.VolumeCreateOptions{Name: scratch, Labels: labels}); err != nil {
		return "", err
	}
	*created = true
	scratchMount := mount.Mount{Type: mount.TypeVolume, Source: scratch, Target: "/scratch"}
	image, err := d.s.deps.HelperImage(d.prepare, d.cli)
	if err != nil {
		return "", err
	}
	helper, err := d.cli.ContainerCreate(d.prepare, client.ContainerCreateOptions{Config: &container.Config{Image: image, Cmd: []string{"sleep", "infinity"}, NetworkDisabled: true, Labels: rustic.OperationLabels(d.prepare)}, HostConfig: volumehelper.HostConfig(image, nil, []mount.Mount{scratchMount})})
	if err != nil {
		return "", err
	}
	if _, err = d.cli.ContainerStart(d.prepare, helper.ID, client.ContainerStartOptions{}); err != nil {
		return helper.ID, err
	}
	if _, _, err = d.s.deps.Exec(d.prepare, helper.ID, "", []string{"mkdir", "-p", "/scratch/data"}); err != nil {
		return helper.ID, err
	}
	return helper.ID, d.s.deps.Engine.RestoreSnapshot(d.prepare, d.cli, d.repository, d.password, d.snapshot, scratchMount, backup.RestoreOptions{DeleteExtra: true, DestinationPath: "/scratch/data"})
}
