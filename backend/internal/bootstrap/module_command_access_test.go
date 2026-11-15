// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Real Edge ingress refuses an incomplete advertised module surface.

package bootstrap

import (
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/features"
	"github.com/stretchr/testify/require"
	"slices"
	"testing"
)

func TestM27M5MissingModuleCommandRefusesBeforeDispatch(t *testing.T) {
	x := newM27ModuleFixture(t, "edge", true)
	require.NoError(t, x.hubSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
	require.NoError(t, x.agentSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
	for _, command := range edge.ModuleCommands(features.Proxy) {
		original := x.tunnel.Capabilities
		x.tunnel.Capabilities = slices.DeleteFunc(slices.Clone(original), func(v string) bool { return v == command })
		require.Equal(t, "agent.update-required", x.hosts.ModuleReason(t.Context(), "example", features.Proxy), command)
		response := x.request("DELETE", m27HostProbe, "")
		require.Equal(t, 409, response.Code, response.Body.String())
		require.Zero(t, x.calls.Load())
		x.tunnel.Capabilities = original
	}
}
