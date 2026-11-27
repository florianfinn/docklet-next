// Source: docklet-next/issues/680
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age implementation.

package project

import (
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestObservedStackExpectedUsesPullableManifest(t *testing.T) {
	for _, store := range []string{"classic", "containerd"} {
		for _, which := range []string{"manifest", "native", "other-service"} {
			t.Run(store+"/"+which, func(t *testing.T) {
				f, _ := observedStackFixture(t)
				if store == "containerd" {
					f.containerd = true
					f.establishBaseline()
				}
				intent := observedStackIntent(t, f, "scheduled", "version")
				tuple := dto.ExpectedCandidate{Service: "app", Manifest: lifecycleDigest}
				switch which {
				case "native":
					tuple.Manifest = lifecycleNewID
				case "other-service":
					tuple.Service = "other"

				}
				intent.Policy.Expected = []dto.ExpectedCandidate{tuple}
				out := f.wait(f.admit(intent))
				if which != "manifest" {
					require.Equal(t, activityDTO.StatusFailed, out.Status)
					require.Equal(t, "update.minimum-age", out.ImageLifecycle.Code)
					require.Equal(t, &dto.RecoveryResult{Outcome: "not-needed"}, out.ImageLifecycle.Recovery)
					require.Empty(t, out.ImageLifecycle.RestorePointID)
					require.Empty(t, out.ImageLifecycle.Applied)
					require.Zero(t, f.pulls)
					require.Zero(t, f.tagWrites)
					require.Zero(t, f.deploys)
					require.Empty(t, lifecyclePoints(t, f))
				} else {
					require.Equal(t, activityDTO.StatusSuccess, out.Status, "%+v", out.ImageLifecycle)
					require.Len(t, out.ImageLifecycle.Applied, 1)
					require.Equal(t, lifecycleDigest, out.ImageLifecycle.Applied[0].Manifest)
					require.Equal(t, 1, f.pulls)
				}
			})
		}
	}
}
