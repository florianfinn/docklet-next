// Source: docklet-next/issues/248
// Revision: 5a4ecc67a7712349cb869574429d09f09a96e895
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original proofs for normalized image reference joins.

package details

import (
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/project"
	"github.com/stretchr/testify/require"
)

// An observation stored under another spelling of the same image joins the service.
func TestMergeJoinsEquivalentImageSpellings(t *testing.T) {
	base := map[string]*imagetypes.UpdateInfo{"docker.io/library/nginx:latest": {HasUpdate: true}}
	result := MergeProjectContainerUpdateInfo(base, []project.RuntimeService{{ContainerID: "web", Image: "nginx"}}, nil)
	require.NotNil(t, result["nginx"])
	require.True(t, result["nginx"].HasUpdate)
}

// An opt-out on one spelling drops the shared result stored under another.
func TestMergeDropsOptedOutEquivalentSpelling(t *testing.T) {
	base := map[string]*imagetypes.UpdateInfo{"nginx:latest": {HasUpdate: true}}
	services := []project.RuntimeService{{ContainerID: "web", Image: "docker.io/library/nginx", ContainerLabels: map[string]string{imageref.UpdateCheckLabel: "false"}}}
	result := MergeProjectContainerUpdateInfo(base, services, nil)
	require.Empty(t, result)
}

func TestUpdateInfoForRefKeepsLiteralFirst(t *testing.T) {
	literal, other := &imagetypes.UpdateInfo{LatestVersion: "literal"}, &imagetypes.UpdateInfo{LatestVersion: "other"}
	byRef := map[string]*imagetypes.UpdateInfo{"nginx": literal, "docker.io/library/nginx:latest": other}
	require.Same(t, literal, UpdateInfoForRef(byRef, " nginx "))
	require.Same(t, other, UpdateInfoForRef(map[string]*imagetypes.UpdateInfo{"docker.io/library/nginx:latest": other}, "library/nginx"))
	require.Nil(t, UpdateInfoForRef(byRef, "nginx:1.27"))
}
