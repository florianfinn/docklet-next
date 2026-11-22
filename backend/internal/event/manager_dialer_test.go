// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original proof that the event HTTP fallback uses the tailnet manager dialer.

package event

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/tailnet/tailnettest"
)

// Without a live tunnel the event falls back to HTTP, which must reach the hub only through the dialer.
func TestEventHTTPFallbackUsesTheManagerDialer(t *testing.T) {
	tailnettest.HostileProxyEnv(t)
	hub := tailnettest.NewSocketHub(t, nil)
	cfg := &config.Config{AgentMode: true, AgentToken: "test-only", ManagerApiUrl: tailnettest.UnreachableManager, ManagerDialer: hub.Dial}
	svc := NewEventService(nil, cfg, nil)
	svc.forwardEventToManager(t.Context(), &Event{Type: "container.start", Severity: EventSeverityInfo, Title: "example"})
	require.Eventually(t, func() bool { return hub.Saw("POST /api/events") }, 5*time.Second, 10*time.Millisecond)
	require.Positive(t, hub.Dials.Load())
}
