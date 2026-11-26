// Source: docklet-next/issues/634
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original build-fact scope of configured service observations.

package project

import (
	"strings"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
	"github.com/stretchr/testify/require"
)

// A service's own row keeps its build facts only on its pinned platform; a
// platform-only Compose edit leaves the row eligible but not its build dates.
func TestConfiguredServiceRecordScopesPinnedPlatform(t *testing.T) {
	built := time.Date(2026, 11, 24, 6, 0, 0, 0, time.UTC)
	ref := "registry.example.com/app:latest"
	for _, pinned := range []string{"", "linux/amd64", "linux/arm64"} {
		t.Run(pinned, func(t *testing.T) {
			service := types.ServiceConfig{Name: "web", Image: ref, Platform: pinned}
			record := imageupdate.ImageUpdateRecord{ID: "service", ProjectID: "project", ServiceName: "web", PolicyKey: imageref.UpdatePolicyKey(ref, nil), Repository: "registry.example.com/app", Tag: "latest", CheckTime: time.Now().UTC(), HasUpdate: true, Platform: new("native-v1:classic:linux/amd64"), CurrentDigest: new("sha256:" + strings.Repeat("a", 64)), LatestDigest: new("sha256:" + strings.Repeat("b", 64)), CurrentCreated: &built, LatestCreated: &built}
			info := BuildConfiguredUpdateInfo("project", []types.ServiceConfig{service}, nil, []imageupdate.ImageUpdateRecord{record}).ServiceUpdates["web"].UpdateInfo
			require.NotNil(t, info)
			require.True(t, info.HasUpdate, "build facts never change the state")
			if pinned == "linux/arm64" {
				require.Empty(t, info.Platform)
				require.Nil(t, info.CurrentCreated)
				require.Nil(t, info.LatestCreated)
				return
			}
			require.Equal(t, "linux/amd64", info.Platform)
			require.NotNil(t, info.LatestCreated)
		})
	}
}
