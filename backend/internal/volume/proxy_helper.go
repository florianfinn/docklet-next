// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine-independent proxy host boundary.

package volume

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/volume/operationlock"
	"github.com/moby/moby/client"
)

// WithProxyHelper retains the existing workspace lock and helper in-use hold.
// Acquisition applies the unchanged bind rules and witness; callers cannot create
// an unguarded helper or evade the reaper. The callback receives no host path.
func (s *VolumeService) WithProxyHelper(ctx context.Context, key string, fn func(context.Context, *client.Client, string) error) error {
	unlock, ok := s.workspaceLocks.TryLock(key)
	if !ok {
		return &volumeWorkspaceBusyError{name: key}
	}
	ctx, release := operationlock.WithContext(ctx, key, unlock)
	defer release()
	id, cleanup, err := s.acquireVolumeHelperInternal(ctx, key)
	if err != nil {
		return err
	}
	defer cleanup()
	if err = s.requireVolumeHelperACFSInternal(ctx, key, id); err != nil {
		return err
	}
	cli, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return err
	}
	return fn(ctx, cli, id)
}
