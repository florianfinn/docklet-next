// Source: docklet-next/issues/634
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original platform scope of reference observations lent to pinned Compose services.

package project

import (
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
)

// The reference observation is made at the daemon's default platform; a
// service pinned to another platform must not show its state or build dates.
func TestConfiguredUpdateInfoScopesPinnedPlatform(t *testing.T) {
	built := time.Date(2026, 11, 24, 6, 0, 0, 0, time.UTC)
	for _, pinned := range []string{"", "linux/amd64", "linux/arm64"} {
		t.Run(pinned, func(t *testing.T) {
			byRef := map[string]*imagetypes.UpdateInfo{"registry.example.com/app:latest": {ImageRef: "registry.example.com/app:latest", State: "rebuild", HasUpdate: true, Platform: "linux/amd64", CheckTime: time.Now().UTC(), CurrentDigest: "sha256:a", LatestDigest: "sha256:b", CurrentCreated: &built, LatestCreated: &built}}
			info := BuildConfiguredUpdateInfo("project", []types.ServiceConfig{{Name: "web", Image: "registry.example.com/app:latest", Platform: pinned}}, byRef, nil).ServiceUpdates["web"].UpdateInfo
			require.NotNil(t, info)
			if pinned == "linux/arm64" {
				require.Equal(t, "unknown", info.State)
				require.False(t, info.HasUpdate)
				require.Empty(t, info.Platform)
				require.Empty(t, info.LatestDigest)
				require.Nil(t, info.CurrentCreated)
				require.Nil(t, info.LatestCreated)
				return
			}
			require.Equal(t, "rebuild", info.State)
			require.NotNil(t, info.LatestCreated)
		})
	}
}
