// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original proof that agent notification dispatch uses the tailnet manager dialer.

package notification

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/tailnet/tailnettest"
	dto "github.com/florianfinn/docklet-next/types/notification"
)

func TestAgentNotificationDispatchUsesTheManagerDialer(t *testing.T) {
	tailnettest.HostileProxyEnv(t)
	hub := tailnettest.NewSocketHub(t, nil)
	cfg := &config.Config{AgentMode: true, AgentToken: "test-only", ManagerApiUrl: tailnettest.UnreachableManager, ManagerDialer: hub.Dial}
	svc := NewNotificationService(nil, cfg, nil, nil)
	out, err := svc.dispatchNotificationToManagerInternal(t.Context(), dto.DispatchRequest{Kind: dto.DispatchKindAutoHeal})
	require.NoError(t, err, "with no tunnel the HTTP dispatch is the only path, so success proves the dialer")
	require.Equal(t, "ok", out.Message)
	require.True(t, hub.Saw("POST /api/notifications/dispatch"))
}
