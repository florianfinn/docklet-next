// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original daemon-host scratch bridge and owned restore effects.

package restore

import (
	"context"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

const RestoreLabel = "docklet.restore-id"
const OperationVolumeLabel = "docklet.backup-volume"

var volumeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]+$`)

type preparedRestore struct {
	repository         backup.Repository
	snapshot, password string
	entry              *volume.Backup
	selection          selectionInternal
	scratch, helper    string
	writers            []string
}

// ValidateAdmission performs bounded metadata checks, without repository effects.
func (s *Service) ValidateAdmission(ctx context.Context, r StartRequest) error {
	if r.Kind != "upload" {
		entry, err := s.deps.Run(ctx, r.BackupID)
		if err != nil || entry.VolumeName != r.VolumeName || entry.LocalSnapshotID == "" {
			return dto.NewProblem(404, "volume.backup.snapshot-not-found", err)
		}
	}
	if r.Mode == "new" && (!volumeNamePattern.MatchString(r.NewVolumeName) || r.NewVolumeName == r.VolumeName) {
		return dto.NewProblem(422, "backup.target.invalid", nil)
	}
	cli, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return dto.NewProblem(503, "volume.backup.engine-unavailable", err)
	}
	target := restoreDestination(r)
	_, err = cli.VolumeInspect(ctx, target, client.VolumeInspectOptions{})
	if r.Mode == "new" && err == nil {
		return dto.NewProblem(409, "volume.backup.busy", nil)
	}
	if err != nil && !errdefs.IsNotFound(err) {
		return dto.NewProblem(503, "volume.backup.engine-unavailable", err)
	}
	if r.Kind == "files" && errdefs.IsNotFound(err) {
		return dto.NewProblem(404, "volume.not-found", nil)
	}
	return nil
}
func restoreDestination(r StartRequest) string {
	if r.Mode == "new" {
		return r.NewVolumeName
	}
	return r.VolumeName
}

func (s *Service) PrepareRestore(ctx context.Context, run *Run, r StartRequest) (cleanup func() error, err error) {
	cli, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	prepared := &preparedRestore{}
	run.Prepared = prepared
	cleanup = func() error {
		for _, writer := range prepared.writers {
			if err := removeConfirmed(ctx, cli, writer); err != nil {
				return err
			}
		}
		if prepared.helper != "" {
			if err := removeConfirmed(ctx, cli, prepared.helper); err != nil {
				return err
			}
		}
		if prepared.scratch != "" {
			_, err := cli.VolumeRemove(context.WithoutCancel(ctx), prepared.scratch, client.VolumeRemoveOptions{})
			if err != nil && !errdefs.IsNotFound(err) {
				return err
			}
		}
		return nil
	}
	if r.Kind == "upload" {
		if err = s.prepareUpload(ctx, cli, run, r, prepared); err != nil {
			return cleanup, err
		}
	} else {
		prepared.entry, err = s.deps.Run(ctx, r.BackupID)
		if err != nil {
			return cleanup, err
		}
		prepared.repository, prepared.snapshot, err = s.deps.Repository(ctx, cli, prepared.entry)
		if err != nil {
			return cleanup, err
		}
		prepared.password, err = s.deps.Password(ctx, cli, prepared.repository)
		if err != nil {
			return cleanup, err
		}
		if r.Kind == "files" {
			prepared.selection, err = s.resolveRestoreSelectionInternal(ctx, cli, prepared.entry, prepared.repository, prepared.snapshot, r.Selection)
			if err != nil {
				return cleanup, err
			}
		}
	}
	target := restoreDestination(r)
	_, err = cli.VolumeInspect(ctx, target, client.VolumeInspectOptions{})
	if err == nil {
		if r.Mode == "new" {
			return cleanup, dto.NewProblem(409, "volume.backup.busy", nil)
		}
		return cleanup, nil
	}
	if !errdefs.IsNotFound(err) {
		return cleanup, err
	}
	if r.Kind == "files" {
		return cleanup, dto.NewProblem(404, "volume.not-found", nil)
	}
	// Intent precedes create. Cleanup verifies the operation label, including when
	// a process dies after Docker created the volume but before the reply arrived.
	run.CreatedVolume = true
	if err = s.lifecycle.persist(ctx, run); err != nil {
		return cleanup, err
	}
	labels := map[string]string{}
	if r.Mode == "new" {
		labels[RestoreLabel] = run.ID
	}
	_, err = cli.VolumeCreate(ctx, client.VolumeCreateOptions{Name: target, Driver: "local", Labels: labels})
	return cleanup, err
}

func (s *Service) prepareUpload(ctx context.Context, cli *client.Client, run *Run, r StartRequest, p *preparedRestore) error {
	input, err := os.Open(r.InputPath)
	if err != nil {
		return err
	}
	defer input.Close()
	validated, err := ValidateArchive(input, dto.MaxExpandedUploadBytes)
	if err != nil {
		return err
	}
	run.InputSHA256 = validated.SHA256
	if err = s.lifecycle.persist(ctx, run); err != nil {
		return err
	}
	p.scratch = "docklet-restore-" + run.ID
	labels := volumehelper.Labels()
	labels[RestoreLabel] = run.ID
	labels[OperationVolumeLabel] = run.VolumeName
	if _, err = cli.VolumeCreate(ctx, client.VolumeCreateOptions{Name: p.scratch, Labels: labels}); err != nil {
		return scratchProblem(err)
	}
	image, err := s.deps.HelperImage(ctx, cli)
	if err != nil {
		return err
	}
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{Image: image, Cmd: []string{"sleep", "infinity"}, Labels: labels}, HostConfig: volumehelper.HostConfig(image, nil, []mount.Mount{{Type: mount.TypeVolume, Source: p.scratch, Target: "/scratch"}})})
	if err != nil {
		return scratchProblem(err)
	}
	p.helper = created.ID
	if _, err = cli.ContainerStart(ctx, p.helper, client.ContainerStartOptions{}); err != nil {
		return scratchProblem(err)
	}
	if _, err = input.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if _, _, err = s.deps.Exec(ctx, p.helper, "", []string{"mkdir", "-p", "/scratch/data"}); err != nil {
		return scratchProblem(err)
	}
	reader, writer := io.Pipe()
	encoded := make(chan error, 1)
	go func() {
		e := NormalizeArchive(input, writer, dto.MaxExpandedUploadBytes, validated.SHA256)
		_ = writer.CloseWithError(e)
		encoded <- e
	}()
	_, copyErr := cli.CopyToContainer(ctx, p.helper, client.CopyToContainerOptions{DestinationPath: "/scratch/data", Content: reader, AllowOverwriteDirWithFile: false, CopyUIDGID: true})
	_ = reader.CloseWithError(copyErr)
	encodeErr := <-encoded
	if encodeErr != nil {
		return encodeErr
	}
	if copyErr != nil {
		return scratchProblem(copyErr)
	}
	_, _, err = s.deps.Exec(ctx, p.helper, "", []string{"tar", "-C", "/scratch/data", "-cf", "/scratch/.docklet-copy.tar", "."})
	return scratchProblem(err)
}
func scratchProblem(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "no space left") || errors.Is(err, syscall.ENOSPC) {
		return archiveProblem("insufficient-space", err)
	}
	return dto.NewProblem(500, "volume.backup.upload-invalid", err)
}

func (s *Service) WriteRestore(ctx context.Context, run *Run, r StartRequest) error {
	cli, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return err
	}
	p := run.Prepared
	target := restoreDestination(r)
	if r.Kind == "upload" {
		image, err := s.deps.HelperImage(ctx, cli)
		if err != nil {
			return err
		}
		labels := volumehelper.Labels()
		labels[RestoreLabel] = run.ID
		labels[OperationVolumeLabel] = run.VolumeName
		created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{Image: image, Cmd: []string{"sleep", "infinity"}, Labels: labels}, HostConfig: volumehelper.HostConfig(image, nil, []mount.Mount{{Type: mount.TypeVolume, Source: p.scratch, Target: "/scratch", ReadOnly: true}, {Type: mount.TypeVolume, Source: target, Target: "/volume"}})})
		if err != nil {
			return err
		}
		p.writers = append(p.writers, created.ID)
		if _, err = cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
			return err
		}
		// Tar is staged inside scratch, never underneath the directory being wiped.
		_, _, err = s.deps.Exec(ctx, created.ID, "", []string{"sh", "-c", `set -e; rm -rf -- /volume/* /volume/.[!.]* /volume/..?*; tar -C /volume -xpf /scratch/.docklet-copy.tar`})
		return err
	}
	if r.Kind == "files" {
		return s.restoreSelectionInternal(ctx, cli, target, r.BackupID, p.entry, p.repository, p.snapshot, p.selection)
	}
	return s.deps.Engine.RestoreSnapshot(ctx, cli, p.repository, p.password, p.snapshot, mount.Mount{Type: mount.TypeVolume, Source: target, Target: "/volume"}, backup.RestoreOptions{DeleteExtra: true})
}
func (s *Service) CleanupNew(ctx context.Context, run *Run) error {
	if !run.CreatedVolume {
		return nil
	}
	cli, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return err
	}
	current, err := cli.VolumeInspect(ctx, run.NewVolumeName, client.VolumeInspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if current.Volume.Labels[RestoreLabel] != run.ID {
		return dto.NewProblem(409, "volume.backup.busy", nil)
	}
	_, err = cli.VolumeRemove(ctx, run.NewVolumeName, client.VolumeRemoveOptions{})
	return err
}
func removeConfirmed(ctx context.Context, cli *client.Client, id string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	_, _ = cli.ContainerRemove(ctx, id, volumehelper.RemoveOptions())
	_, err := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("helper removal unconfirmed")
}
