// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package environment

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	environmenttypes "github.com/florianfinn/docklet-next/types/environment"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAgentManualTokenChangeRejectsLateTunnel(t *testing.T) {
	for _, transport := range []string{"service", "middleware"} {
		t.Run(transport, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			f := newAgentFixture(t, "edge", version.Info{})
			old := f.tunnel
			replacement := "test-only"
			_, err := f.service.UpdateEnvironment(ctx, f.id, map[string]any{"access_token": replacement}, nil, nil)
			require.NoError(t, err)
			// The old credential finished authentication before update but registered afterwards.
			f.conn.mu.Lock()
			f.conn.closed = false
			f.conn.mu.Unlock()
			edge.GetRegistry().Register(f.id, old)
			if transport == "service" {
				_, err = f.service.ExecuteRemoteRequest(ctx, f.id, "GET", "/api/app-version", nil)
				require.Error(t, err)
			} else {
				router := echo.New()
				resolver := func(context.Context, string) (string, *string, bool, error) {
					return "edge://example", &replacement, true, nil
				}
				router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", resolver, nil, nil, edge.GetRegistry(), nil, middleware.EnvironmentProxySecurity{AgentAccess: f.service.AgentAccess}))
				router.HEAD("/api/environments/:id/system/health", func(c *echo.Context) error { return c.NoContent(500) })
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest("HEAD", "/api/environments/"+f.id+"/system/health", nil).WithContext(ctx))
				require.NotEqual(t, http.StatusOK, rec.Code)
			}
			require.True(t, f.conn.IsClosed())
			require.Empty(t, f.calls, "the replacement token must not reach a tunnel authenticated with the old token")
			dto := environmenttypes.Environment{ID: f.id, IsEdge: true, Status: "online"}
			f.service.ApplyHostRuntimeState(ctx, &dto)
			require.False(t, *dto.Connected)
		})
	}
}

func TestStaleAgentRequestDoesNotCloseCurrentTunnel(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	stale := edge.WithSessionCredential(t.Context(), "old-key", "test-only", f.service.edgeSessionCurrent)
	require.Error(t, edge.RequireSessionKey(stale, f.tunnel))
	require.False(t, f.conn.IsClosed(), "a stale request must not close the current tunnel")
	require.Empty(t, f.calls)
	fresh := edge.WithSessionCredential(t.Context(), "example", f.token, f.service.edgeSessionCurrent)
	require.NoError(t, edge.RequireSessionKey(fresh, f.tunnel))
	f.tunnel.APIKeyID = "test-only"
	require.Error(t, edge.RequireSessionKey(fresh, f.tunnel))
	require.True(t, f.conn.IsClosed(), "a fresh read confirmed that this tunnel is obsolete")
}

func TestAgentMismatchReadFailureDoesNotCloseTunnel(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	ctx := edge.WithSessionCredential(t.Context(), "old-key", f.token, func(context.Context, *edge.AgentTunnel) (bool, error) {
		return false, errors.New("database unavailable")
	})
	require.Error(t, edge.RequireSessionKey(ctx, f.tunnel))
	require.False(t, f.conn.IsClosed())
}
