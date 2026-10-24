// Source: docklet-next/issues/221
// Revision: 75e73d5c4ffcd78b47ebaa166579079c02f70c8f
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original API-only create refusal; internal Docker create stays idempotent.

package volume

import (
	"context"
	"fmt"

	volumeutils "github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumes"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/client"
)

// Only explicit API creates refuse occupied names; Compose and internal callers
// retain Docker's idempotent create. External creation after inspect can still race.
func (s *VolumeService) createVolumeForAPI(ctx context.Context, options client.VolumeCreateOptions, user usertypes.Actor) (*volumetypes.Volume, error) {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}
	if options.Name != "" {
		if err := volumeutils.EnsureRenameTargetAbsent(ctx, dockerClient, options.Name); err != nil {
			return nil, err
		}
	}
	return s.CreateVolume(ctx, options, user)
}
