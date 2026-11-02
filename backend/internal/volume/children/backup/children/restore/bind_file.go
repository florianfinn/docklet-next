// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package restore

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/volume/bindpath"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"path"
)

func (s *Service) bindSourceMount(ctx context.Context, cli *client.Client, key string) (mount.Mount, string, error) {
	source, err := bindpath.Check(ctx, cli, s.deps.DB, key)
	if err != nil {
		return mount.Mount{}, "", err
	}
	image, err := s.deps.HelperImage(ctx, cli)
	if err != nil {
		return mount.Mount{}, "", err
	}
	kind, err := bindpath.Witness(ctx, cli, image, source, s.deps.Exec)
	if err != nil {
		return mount.Mount{}, "", err
	}
	return bindpath.SourceMount(source, kind, false), kind, nil
}

// BindEffects wires the single-file phases: the scratch root check runs in preparing, before any
// live write, and the in-place copy runs in writing.
func (s *Service) BindEffects(e *LifecycleEffects) {
	e.PostSafety, e.Write = s.PrepareBindFile, s.WriteRestore
}

// PrepareBindFile is a preparing-phase effect after safety capture and before live writes.
func (s *Service) PrepareBindFile(ctx context.Context, run *Run, r StartRequest) error {
	p := run.Prepared
	if p == nil || p.bindKind != "file" {
		return nil
	}
	cli, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return err
	}
	p.scratch = "docklet-restore-" + run.ID
	labels := volumehelper.Labels()
	labels[RestoreLabel] = run.ID
	labels[OperationVolumeLabel] = run.VolumeName
	if _, err = cli.VolumeCreate(ctx, client.VolumeCreateOptions{Name: p.scratch, Labels: labels}); err != nil {
		return err
	}
	scratch := mount.Mount{Type: mount.TypeVolume, Source: p.scratch, Target: "/scratch"}
	if err = s.deps.Engine.RestoreSnapshot(ctx, cli, p.repository, p.password, p.snapshot, scratch, backup.RestoreOptions{DestinationPath: "/scratch"}); err != nil {
		return err
	}
	image, err := s.deps.HelperImage(ctx, cli)
	if err != nil {
		return err
	}
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{Image: image, Cmd: []string{"sleep", "infinity"}, Labels: labels}, HostConfig: volumehelper.HostConfig(image, nil, []mount.Mount{scratch})})
	if err != nil {
		return err
	}
	p.helper = created.ID
	if _, err = cli.ContainerStart(ctx, p.helper, client.ContainerStartOptions{}); err != nil {
		return err
	}
	source, _ := volume.BindSource(run.VolumeName)
	_, _, err = s.deps.Exec(ctx, p.helper, "", []string{"sh", "-c", bindScratchCheck, "sh", path.Base(source)})
	if err != nil {
		return dto.NewProblem(422, "volume.bind.snapshot-kind-mismatch", err)
	}
	return nil
}

const bindScratchCheck = `set -e
count=0
for entry in /scratch/* /scratch/.[!.]* /scratch/..?*; do
 if ! test -e "$entry" && ! test -L "$entry"; then continue; fi
 count=$((count + 1))
 test "$entry" = "/scratch/$1"
 test -f "$entry"
 test ! -L "$entry"
done
test "$count" = 1`

func (s *Service) writeBindFile(ctx context.Context, cli *client.Client, run *Run, target mount.Mount) error {
	p := run.Prepared
	image, err := s.deps.HelperImage(ctx, cli)
	if err != nil {
		return err
	}
	labels := volumehelper.Labels()
	labels[RestoreLabel] = run.ID
	labels[OperationVolumeLabel] = run.VolumeName
	mounts := []mount.Mount{{Type: mount.TypeVolume, Source: p.scratch, Target: "/scratch", ReadOnly: true}, target}
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{Image: image, Cmd: []string{"sleep", "infinity"}, Labels: labels}, HostConfig: volumehelper.HostConfig(image, nil, mounts)})
	if err != nil {
		return err
	}
	p.writers = append(p.writers, created.ID)
	if _, err = cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return err
	}
	_, _, err = s.deps.Exec(ctx, created.ID, "", []string{"sh", "-c", bindpath.RestoreFileScript, "sh", path.Join("/scratch", path.Base(target.Source)), target.Target})
	return err
}
