// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original confirmed backup helper cleanup under operation ownership.

package backup

import (
	"context"
	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/moby/moby/client"
	"time"
)

// Caller ownership outlives cancellation and a successful remove reply. Only a
// not-found inspect proves that a helper can no longer write the mounted data.
func waitForBackupHelperRemoval(ctx context.Context, cli *client.Client, id string) {
	for {
		attempt, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		_, _ = cli.ContainerRemove(attempt, id, volumehelper.RemoveOptions())
		_, err := cli.ContainerInspect(attempt, id, client.ContainerInspectOptions{})
		cancel()
		if errdefs.IsNotFound(err) {
			return
		}
		time.Sleep(time.Second)
	}
}
