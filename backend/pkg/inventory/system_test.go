// Source: docklet-next/issues/83
// Revision: b15b60e94f1b0bd361d32a03477fdf82647ad108
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original shared-resource visibility tests.

package inventory

import (
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/volume"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSharedResourcesAndBuiltinNetworksStayVisible(t *testing.T) {
	containers := []container.Summary{
		{ID: "agent", ImageID: "shared", Labels: map[string]string{SystemLabel: "agent"}, Mounts: []container.MountPoint{{Type: mount.TypeVolume, Name: "shared"}}, NetworkSettings: &container.NetworkSettingsSummary{Networks: map[string]*network.EndpointSettings{"shared": {NetworkID: "shared"}, "bridge": {NetworkID: "bridge"}}}},
		{ID: "user", ImageID: "shared", State: container.StateExited, Mounts: []container.MountPoint{{Type: mount.TypeVolume, Name: "shared"}}, NetworkSettings: &container.NetworkSettingsSummary{Networks: map[string]*network.EndpointSettings{"shared": {NetworkID: "shared"}}}},
	}
	require.Len(t, Images([]image.Summary{{ID: "shared"}}, containers), 1)
	require.Len(t, Volumes([]volume.Volume{{Name: "shared"}}, containers), 1)
	require.Len(t, Networks([]network.Summary{{ID: "shared", Name: "shared"}, {ID: "bridge", Name: "bridge"}}, containers), 2)
}
