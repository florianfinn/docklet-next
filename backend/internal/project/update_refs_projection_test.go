// Source: docklet-next/issues/248
// Revision: 5a4ecc67a7712349cb869574429d09f09a96e895
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original proof that the configured projection joins equivalent spellings.

package project

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
)

// A digest observation stored under the fully qualified spelling reaches a
// service that names the familiar spelling.
func TestConfiguredUpdateInfoJoinsEquivalentSpelling(t *testing.T) {
	byRef := map[string]*imagetypes.UpdateInfo{"docker.io/library/nginx:latest": {HasUpdate: true, State: "rebuild"}}
	info := BuildConfiguredUpdateInfo("project", []types.ServiceConfig{{Name: "web", Image: "nginx"}}, byRef, nil)
	require.NotNil(t, info.ServiceUpdates["web"].UpdateInfo)
	require.True(t, info.ServiceUpdates["web"].UpdateInfo.HasUpdate)
}
