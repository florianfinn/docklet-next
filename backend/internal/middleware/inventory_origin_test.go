// Source: docklet-next/issues/314
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original hub origin overlay through real Direct and Edge inventory senders.

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	dto "github.com/florianfinn/docklet-next/types/container"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestOriginOverlayRealDirectAndEdgeListAndDetail(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "origins.db")), database.MigrationOptions{})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			store := &origin.Store{DB: db.DB, LocalRegistration: "example"}
			require.NoError(t, store.Assign(t.Context(), "0", origin.Identity{ID: "old", Name: "app", Labels: map[string]string{"com.docker.compose.service": "app"}}, dto.OriginAssignment{Origin: dto.OriginStandalone, DockerID: "old"}, "operator"))
			agent := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				row := `{"id":"external","name":"app","names":["/app"],"origin":"unknown","originSource":"detected","labels":{"com.docker.compose.service":"app"}}`
				if r.URL.Path == "/api/environments/0/containers" {
					_, _ = w.Write([]byte(`{"success":true,"data":[` + row + `]}`))
				} else {
					_, _ = w.Write([]byte(`{"success":true,"data":` + row + `}`))
				}
			})
			server := httptest.NewServer(agent)
			t.Cleanup(server.Close)
			mw := newTestEnvironmentMiddleware()
			mw.snapshots = &hoststate.Store{}
			mw.originOverlay = store.Overlay
			endpoint := server.URL
			token := uuid.NewString()
			if mode == "edge" {
				connected := make(chan struct{})
				var once sync.Once
				tunnelServer := edge.NewTunnelServerWithRegistry(mw.registry, func(context.Context, string) (string, error) { return "example", nil }, func(_ context.Context, _ string, online bool) {
					if online {
						once.Do(func() { close(connected) })
					}
				})
				tunnelServer.SetSessionKeyResolver(func(context.Context, string, string) (string, error) { return "example-key", nil })
				router := echo.New()
				router.GET("/api/tunnel/connect", tunnelServer.HandleConnect)
				manager := httptest.NewServer(router)
				t.Cleanup(manager.Close)
				life, cancel := context.WithCancel(t.Context())
				done := make(chan struct{})
				client := edge.NewRuntimeTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: token, EdgeReconnectInterval: 1}, agent)
				go func() { defer close(done); client.StartWithErrorChan(life, nil) }()
				t.Cleanup(func() {
					cancel()
					mw.registry.Unregister("example")
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Error("Edge client did not join")
					}
				})
				select {
				case <-connected:
				case <-time.After(5 * time.Second):
					t.Fatal("Edge registration barrier not reached")
				}
				endpoint = "edge://example"
				mw.agentAccess = func(ctx context.Context, _ string, _ *string, _, _ string) (context.Context, context.CancelFunc, error) {
					return edge.WithSessionCredential(ctx, "example-key", token, nil), func() {}, nil
				}
			}
			mw.resolver = func(context.Context, string) (string, *string, bool, error) { return endpoint, &token, true, nil }
			router := echo.New()
			group := attachMiddleware(router, mw)
			group.Any("/environments/:id/*", func(c *echo.Context) error { return c.NoContent(418) })
			for _, suffix := range []string{"/containers", "/containers/external"} {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/environments/example"+suffix, nil))
				require.Equal(t, 200, response.Code, response.Body.String())
				require.Contains(t, response.Body.String(), `"originSource":"unconfirmed"`)
				require.Contains(t, response.Body.String(), `"origin":"unknown"`)
				cached, ok := mw.snapshots.Get(hoststate.Key{HostID: "example", List: suffix + "?"})
				require.True(t, ok)
				require.Contains(t, string(cached.Data), `"originSource":"unconfirmed"`)
			}
		})
	}
}
