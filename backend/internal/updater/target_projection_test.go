// Source: docklet-next/issues/634
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original build-fact scope of container check projections.

package updater

import (
	"testing"
	"time"

	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
	"github.com/stretchr/testify/require"
)

// A target missing from the check's container results falls back to the
// image-level observation, which names no build of that container (#634).
func TestContainerCheckProjectionKeepsBuildFactsWithTheirContainer(t *testing.T) {
	built := time.Date(2026, 11, 24, 6, 0, 0, 0, time.UTC)
	own := time.Date(2026, 11, 25, 6, 0, 0, 0, time.UTC)
	response := &checkDTO.Response{ImageRef: "registry.example.com/app:latest", State: "rebuild", HasUpdate: true, Platform: "linux/amd64", CurrentCreated: &built, LatestCreated: &built, ContainerUpdates: map[string]*checkDTO.Response{"present": {ImageRef: "registry.example.com/app:latest", State: "rebuild", HasUpdate: true, Platform: "linux/amd64", LatestCreated: &own}}}
	for id, want := range map[string]*time.Time{"present": &own, "removed": nil} {
		got, err := containerCheckProjection(updateTarget{Name: "app", ContainerID: id, ImageRef: response.ImageRef}, response)
		require.NoError(t, err)
		info := got.ServiceUpdates["app"].UpdateInfo
		require.NotNil(t, info)
		require.Equal(t, want, info.LatestCreated, id)
		if want == nil {
			require.Nil(t, info.CurrentCreated)
			require.Equal(t, "linux/amd64", info.Platform, "candidate bookkeeping keeps its platform")
		}
	}
}
