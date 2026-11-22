// Source: docklet-next/issues/573
// Revision: 6380af76c5b004037da6c1063fe6dde709ff5a4a
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original proof that only an explicit tailnet opt-in replaces an Edge agent's ordinary route (M36-S4A-R08).

package bootstrap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/config"
)

func TestOrdinaryEdgeAgentKeepsNormalNetworking(t *testing.T) {
	cfg := &config.Config{AgentMode: true, EdgeAgent: true, TailscaleStateDir: t.TempDir()}
	route := newAgentTailnetRouteInternal(cfg)
	require.False(t, route.Enabled(), "no key, explicit hostname or retained identity: the ordinary route stays")
	require.Nil(t, cfg.ManagerDialer)
}

func TestExplicitTailnetHostnameEnablesFailClosedRoute(t *testing.T) {
	cfg := &config.Config{AgentMode: true, EdgeAgent: true, TailscaleHostname: "docklet-example", TailscaleStateDir: t.TempDir()}
	route := newAgentTailnetRouteInternal(cfg)
	require.True(t, route.Enabled())
	require.NotNil(t, cfg.ManagerDialer)
	conn, err := cfg.ManagerDialer(context.Background(), "tcp", "192.0.2.1:3552")
	require.Error(t, err, "before the join the manager dialer fails closed")
	require.Nil(t, conn)
}
