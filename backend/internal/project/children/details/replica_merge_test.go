// Source: docklet-next/issues/341
// Revision: 0c8a014eda72706e48d60c22e86526e7dfd15964
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Replicas of one image merge to one coherent projected observation.

package details

import (
	"testing"
	"time"

	imagetypes "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/project"
	"github.com/stretchr/testify/require"
)

func projectedObservation(current, latest string, checked time.Time) *imagetypes.UpdateInfo {
	info := &imagetypes.UpdateInfo{ImageRef: "docker.io/library/alpine:3.22", Platform: "native-v1:classic:linux/amd64", CurrentVersion: "3.22", CurrentDigest: current, LatestDigest: latest, CheckTime: checked}
	imagetypes.ProjectUpdate(info)
	return info
}

func TestReplicaMergeKeepsOneCoherentObservation(t *testing.T) {
	early, late := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC), time.Date(2026, 11, 5, 11, 0, 0, 0, time.UTC)
	a, b, c := "sha256:"+repeat("a"), "sha256:"+repeat("b"), "sha256:"+repeat("c")
	services := []project.RuntimeService{{ContainerID: "one", Image: "alpine:3.22"}, {ContainerID: "two", Image: "alpine:3.22"}}
	t.Run("current and outdated replica", func(t *testing.T) {
		scoped := map[string]*imagetypes.UpdateInfo{"one": projectedObservation(b, b, late), "two": projectedObservation(a, b, early)}
		merged := MergeProjectContainerUpdateInfo(nil, services, scoped)["alpine:3.22"]
		require.NotNil(t, merged)
		require.Equal(t, *scoped["two"], *merged, "the replica that needs its update is returned unchanged")
	})
	t.Run("conflicting candidates", func(t *testing.T) {
		scoped := map[string]*imagetypes.UpdateInfo{"one": projectedObservation(a, b, early), "two": projectedObservation(a, c, late)}
		merged := MergeProjectContainerUpdateInfo(nil, services, scoped)["alpine:3.22"]
		require.NotNil(t, merged)
		require.True(t, merged.HasUpdate)
		require.Equal(t, "rebuild", merged.State)
		require.Empty(t, merged.LatestDigest, "replicas that disagree choose no candidate")
		require.Equal(t, late, merged.CheckTime)
		require.Equal(t, merged.CheckTime, merged.CheckedAt)
	})
}

func repeat(s string) string {
	out := ""
	for range 64 {
		out += s
	}
	return out
}
