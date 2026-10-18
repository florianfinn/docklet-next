// Source: docklet-next/issues/85
// Revision: 0927d517e029fffe31805211b3c02e49c4a93af5
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original immediate pending-inventory and authorization regression tests.

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestAB2PendingInventoryIsImmediateWithoutAgentIO(t *testing.T) {
	for _, connection := range []string{"direct", "edge-disconnected", "edge-connected"} {
		for _, snapshot := range []bool{false, true} {
			t.Run(connection+"/snapshot="+map[bool]string{false: "no", true: "yes"}[snapshot], func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					registry := edge.NewTunnelRegistry()
					apiURL := "http://agent.example.com"
					if connection != "direct" {
						apiURL = "edge://example"
					}
					if connection == "edge-connected" {
						registry.Register("example", edge.NewAgentTunnelWithConn("example", edge.NewGRPCManagerTunnelConn(nil)))
					}
					for _, suffix := range []string{"containers", "projects", "images", "volumes", "networks", "ports", "networks/topology", "dashboard", "projects/app", "volumes/data", "networks/front"} {
						store := &hoststate.Store{}
						key := hoststate.Key{HostID: "example", List: "/" + suffix + "?limit=100"}
						var saved hoststate.Snapshot
						if snapshot {
							saved = store.Put(key, []byte(`{"success":true,"data":[{"id":"saved"}]}`))
						}
						agentCalls := 0
						m := &EnvironmentMiddleware{snapshots: store, registry: registry,
							inventoryPending: func(_ context.Context, id string) (bool, error) {
								require.Equal(t, "example", id)
								return true, nil
							},
							agentAccess: func(context.Context, string, *string, string, string) (context.Context, context.CancelFunc, error) {
								agentCalls++
								return nil, nil, context.DeadlineExceeded
							},
						}
						rec := httptest.NewRecorder()
						c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/api/environments/example/"+suffix+"?limit=100", nil), rec)
						started := time.Now()
						require.NoError(t, m.inventoryRead(c, "example", func() error { return m.forwardInventoryRequest(c, "example", apiURL, nil) }))
						require.Zero(t, time.Since(started), suffix)
						require.Zero(t, agentCalls, suffix)
						require.Equal(t, "agent.unavailable", rec.Header().Get("X-Docklet-Host-Reason"), suffix)
						if snapshot {
							require.Equal(t, http.StatusOK, rec.Code, suffix)
							require.Equal(t, string(saved.Data), rec.Body.String(), suffix)
							require.Equal(t, saved.ObservedAt.Format(time.RFC3339Nano), rec.Header().Get("X-Docklet-Observed-At"), suffix)
						} else {
							require.Equal(t, http.StatusBadGateway, rec.Code, suffix)
							require.Empty(t, rec.Header().Get("X-Docklet-Observed-At"), suffix)
						}
					}
				})
			})
		}
	}
}

func TestAB2PendingInventoryRequiresAuthentication(t *testing.T) {
	router := echo.New()
	router.Use(NewEnvProxyMiddlewareWithParamAndRegistry("0", "id",
		func(context.Context, string) (string, *string, bool, error) {
			t.Fatal("unauthenticated read must not resolve a host")
			return "", nil, false, nil
		},
		func(context.Context, *echo.Context) (*authz.PermissionSet, *usertypes.Actor, bool) {
			return nil, nil, false
		},
		nil, nil, nil, EnvironmentProxySecurity{Snapshots: &hoststate.Store{}, InventoryPending: func(context.Context, string) (bool, error) {
			t.Fatal("unauthenticated read must not inspect pending inventory")
			return true, nil
		}}))
	router.GET("/api/environments/:id/containers", func(c *echo.Context) error { return c.NoContent(500) })
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/environments/example/containers", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
