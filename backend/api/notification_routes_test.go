// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package api

import (
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNotificationsAndAutoHealRouteModes(t *testing.T) {
	for _, mode := range []struct {
		name string
		cfg  *config.Config
		hub  bool
	}{{"hub", &config.Config{}, true}, {"agent", &config.Config{AgentMode: true}, false}, {"edge", &config.Config{EdgeAgent: true}, false}} {
		t.Run(mode.name, func(t *testing.T) {
			api := setupAPIForSpecMode(mode.cfg)
			require.Equal(t, mode.hub, api.OpenAPI().Paths["/environments/{id}/notifications/settings"] != nil)
			require.Equal(t, mode.hub, api.OpenAPI().Paths["/notifications/dispatch"] != nil)
			require.NotNil(t, api.OpenAPI().Paths["/environments/{id}/auto-heal"].Get)
			require.NotNil(t, api.OpenAPI().Paths["/environments/{id}/auto-heal"].Put)
		})
	}
	for _, method := range []string{"GET", "PUT"} {
		require.True(t, edge.ResolveEdgeCommandName(method, "/api/environments/0/auto-heal", false).IsPresent())
	}
	require.False(t, edge.ResolveEdgeCommandName("GET", "/api/environments/0/notifications/settings", false).IsPresent())
}
