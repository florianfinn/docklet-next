// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original generic volume guard for credential-bearing backup mounts.

package volume

import (
	"context"
	"github.com/containerd/errdefs"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/moby/moby/client"
)

func isBackupTargetVolume(labels map[string]string) bool {
	_, target := labels[dto.TargetVolumeLabel]
	return target
}
func hiddenTargetVolumeError() error { return dto.NewProblem(404, "volume.not-found", nil) }
func refuseTargetVolumeName(ctx context.Context, cli *client.Client, name string) error {
	if name == "" {
		return nil
	}
	inspected, err := cli.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if isBackupTargetVolume(inspected.Volume.Labels) {
		return hiddenTargetVolumeError()
	}
	return nil
}
