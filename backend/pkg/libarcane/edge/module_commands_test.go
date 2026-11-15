// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original complete optional-module command advertisement proof.

package edge

import (
	"github.com/florianfinn/docklet-next/types/features"
	"github.com/stretchr/testify/require"
	"slices"
	"testing"
)

func TestModuleCommandsRequiresEveryCommand(t *testing.T) {
	all := AdvertisedEdgeCommands()
	require.True(t, HasModuleCommands(all, features.Proxy))
	for _, command := range ModuleCommands(features.Proxy) {
		missing := slices.DeleteFunc(slices.Clone(all), func(v string) bool { return v == command })
		require.False(t, HasModuleCommands(missing, features.Proxy), command)
	}
	require.False(t, HasModuleCommands(all, "unknown"))
}
