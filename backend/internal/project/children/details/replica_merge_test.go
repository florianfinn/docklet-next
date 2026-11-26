// Source: docklet-next/issues/341
// Revision: 0c8a014eda72706e48d60c22e86526e7dfd15964
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Replicas of one image merge to one coherent projected observation; #634 build times.

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
		scoped["one"].LatestCreated, scoped["two"].LatestCreated = &early, &late
		merged := MergeProjectContainerUpdateInfo(nil, services, scoped)["alpine:3.22"]
		require.NotNil(t, merged)
		require.True(t, merged.HasUpdate)
		require.Equal(t, "rebuild", merged.State)
		require.Empty(t, merged.LatestDigest, "replicas that disagree choose no candidate")
		require.Nil(t, merged.LatestCreated, "nor a candidate build time")
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

// Replicas observed on different platforms share no build facts; a lent side
// borrows them only from an observation of the same native build (#634).
func TestReplicaMergeScopesBuildFacts(t *testing.T) {
	early, late := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC), time.Date(2026, 11, 5, 11, 0, 0, 0, time.UTC)
	a, b, c := "sha256:"+repeat("a"), "sha256:"+repeat("b"), "sha256:"+repeat("c")
	services := []project.RuntimeService{{ContainerID: "one", Image: "alpine:3.22"}, {ContainerID: "two", Image: "alpine:3.22"}}
	amd, arm := projectedObservation(a, b, late), projectedObservation(c, c, early)
	amd.Platform, arm.Platform = "linux/amd64", "linux/arm64"
	amd.CurrentCreated, amd.LatestCreated = &early, &late
	merged := MergeProjectContainerUpdateInfo(nil, services, map[string]*imagetypes.UpdateInfo{"one": amd, "two": arm})["alpine:3.22"]
	require.NotNil(t, merged)
	require.True(t, merged.HasUpdate)
	require.Empty(t, merged.Platform, "one platform's build names no build of the other")
	require.Nil(t, merged.CurrentCreated)
	require.Nil(t, merged.LatestCreated)

	lent := imagetypes.Lend(projectedObservation(a, b, late))
	owned := projectedObservation(a, b, early)
	owned.Platform, owned.CurrentCreated, owned.LatestCreated = "linux/amd64", &early, &late
	merged = MergeProjectContainerUpdateInfo(nil, services, map[string]*imagetypes.UpdateInfo{"one": lent, "two": owned})["alpine:3.22"]
	require.NotNil(t, merged)
	require.Equal(t, late, merged.CheckTime)
	require.Equal(t, "linux/amd64", merged.Platform, "the same native build lends its facts")
	require.NotNil(t, merged.LatestCreated)
}
