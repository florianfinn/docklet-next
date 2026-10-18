// Source: docklet-next/issues/85
// Revision: a97d94423e731203eaf5bdcfe96e7744ce592fa2
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original bounded Edge recovery tests with the Go fake clock.

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestAN1ColdEdgeInventoryRecovery(t *testing.T) {
	for _, scenario := range []string{"reconnect", "timeout", "cancel", "snapshot"} {
		t.Run(scenario, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				registry := edge.NewTunnelRegistry()
				store := &hoststate.Store{}
				m := &EnvironmentMiddleware{snapshots: store, registry: registry}
				checked := false
				m.agentAccess = func(ctx context.Context, _ string, _ *string, _, _ string) (context.Context, context.CancelFunc, error) {
					checked = true
					require.True(t, m.hasActiveEdgeTunnelInternal("example"))
					return nil, nil, remenv.ErrAgentUpdateRequired
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				recoveryDone := make(chan struct{})
				switch scenario {
				case "reconnect":
					go func() {
						defer close(recoveryDone)
						time.Sleep(400 * time.Millisecond)
						registry.Register("example", edge.NewAgentTunnelWithConn("example", edge.NewGRPCManagerTunnelConn(nil)))
					}()
				case "cancel":
					go func() { defer close(recoveryDone); time.Sleep(150 * time.Millisecond); cancel() }()
				case "snapshot":
					close(recoveryDone)
					store.Put(hoststate.Key{HostID: "example", List: "/containers?limit=100"}, []byte(`{"success":true,"data":[{"id":"cached"}]}`))
				default:
					close(recoveryDone)
				}
				rec := httptest.NewRecorder()
				c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/api/environments/example/containers?limit=100", nil).WithContext(ctx), rec)
				started := time.Now()
				require.NoError(t, m.inventoryRead(c, "example", func() error { return m.forwardInventoryRequest(c, "example", "edge://example", nil) }))
				elapsed := time.Since(started)
				<-recoveryDone
				switch scenario {
				case "reconnect":
					require.True(t, checked, "recovered tunnel must reach the compatibility gate")
					require.GreaterOrEqual(t, elapsed, 400*time.Millisecond)
					require.Less(t, elapsed, time.Second)
					require.Equal(t, http.StatusConflict, rec.Code)
					require.Contains(t, rec.Body.String(), "agent.update-required")
				case "timeout":
					require.False(t, checked)
					require.Equal(t, time.Second, elapsed)
					require.Equal(t, http.StatusBadGateway, rec.Code)
				case "cancel":
					require.False(t, checked)
					require.Equal(t, 150*time.Millisecond, elapsed)
					require.Equal(t, http.StatusBadGateway, rec.Code)
				case "snapshot":
					require.False(t, checked)
					require.Zero(t, elapsed)
					require.Equal(t, http.StatusOK, rec.Code)
					require.Contains(t, rec.Body.String(), "cached")
				}
			})
		})
	}
}
