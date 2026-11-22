// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original agent join and late-bound dialer tests.

package tailnet_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/tailnet"
	"github.com/florianfinn/docklet-next/backend/internal/tailnet/tailnettest"
)

// Control unreachable for the first attempts, then reachable: the agent joins without a reinstall.
func TestAgentJoinRetriesUntilControlIsReachable(t *testing.T) {
	fleet := tailnettest.NewFleet()
	fleet.FailNextUps(context.DeadlineExceeded, context.DeadlineExceeded)
	route := tailnet.NewAgentRoute(tailnet.AgentConfig{
		AuthKey: tailnettest.Key("auth", "example-host"), Hostname: "docklet-example", StateDir: t.TempDir(),
		Factory: fleet.Factory, RetryMin: 5 * time.Millisecond, RetryMax: 20 * time.Millisecond,
	})
	require.True(t, route.Enabled())

	_, err := route.Dial(t.Context(), "tcp", "192.0.2.10:3552")
	require.Error(t, err, "before the first join no hub connection falls back to normal routing")

	require.NoError(t, route.Join(t.Context()))
	require.EqualValues(t, 3, fleet.Created.Load())
	nodes := fleet.Nodes()
	require.Empty(t, nodes[2].Config.AdvertiseTags, "agents take their tag from the key")
	require.Equal(t, "docklet-example", nodes[2].Config.Hostname)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	conn, err := route.Dial(t.Context(), "tcp", listener.Addr().String())
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	require.NoError(t, route.Close())
	require.True(t, nodes[2].Closed())
}

func TestAgentRouteIsOffWithoutAuthKey(t *testing.T) {
	require.False(t, tailnet.NewAgentRoute(tailnet.AgentConfig{}).Enabled())
	var nilRoute *tailnet.AgentRoute
	require.False(t, nilRoute.Enabled())
	require.NoError(t, nilRoute.Close())
}
