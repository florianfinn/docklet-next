// Source: docklet-next/issues/492
// Revision: 52bae4d0610f04e66451664e2c0741699c901873
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original real Huma stub routes and Direct/Edge v19 admission proof.

package proxy

import (
	"context"
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func contractRouter(m *Module, agent bool) *echo.Echo {
	router := echo.New()
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("contract", "test"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, &authz.PermissionSet{Sudo: true})))
	})
	m.RegisterRoutes(api, agent)
	return router
}
func postContract(router http.Handler, path string, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", path, strings.NewReader(string(raw)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}
func TestM28HubAndAgentStubRoutes(t *testing.T) {
	x := newFixture(t)
	instance := x.request.Instance
	instance.EnvironmentID = "0"
	instance.Authority = dto.FileAuthority
	m := &Module{Agent: x.a, Settings: x.settings, Modules: x.mods, Hub: &Hub{DB: x.db.DB}}
	m.Transport = &HostTransport{Local: m}
	require.NoError(t, m.saveInstance(t.Context(), instance))

	certificate := dto.CertificateCheckRequest{File: "routes/app.yml", Router: "app", SourceHash: strings.Repeat("a", 64), Host: "app.example.test"}
	hub, agent := contractRouter(m, false), contractRouter(m, true)
	for _, tt := range []struct {
		router http.Handler
		path   string
		body   any
	}{
		{hub, "/api/modules/proxy/instances/instance-one/certificate-check", certificate},
		{agent, "/api/environments/0/modules/proxy/certificate-check", dto.HostCertificateCheckRequest{Instance: instance, CertificateCheckRequest: certificate}},
	} {
		w := postContract(tt.router, tt.path, tt.body)
		if strings.HasSuffix(tt.path, "certificate-check") {
			require.Equal(t, 409, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "certificate-unavailable")
		} else {
			require.Equal(t, 501, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "not-implemented")
		}
	}
	for _, table := range []string{"proxy_journal", "proxy_operations", "proxy_reservations"} {
		var n int64
		require.NoError(t, x.db.Table(table).Count(&n).Error)
		require.Zero(t, n)
	}
}

type contractConn struct{ send func(*edge.TunnelMessage) }

func (c *contractConn) Send(m *edge.TunnelMessage) error    { c.send(m); return nil }
func (*contractConn) Receive() (*edge.TunnelMessage, error) { return nil, io.EOF }
func (*contractConn) IsExpectedReceiveError(error) bool     { return true }
func (*contractConn) Close() error                          { return nil }
func (*contractConn) IsClosed() bool                        { return false }
func (*contractConn) Transport() string                     { return edge.EdgeTransportWebSocket }

func TestM28V19NoNewModuleRequests(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			x := newFixture(t)
			var calls atomic.Int32
			respond := func(method, path string) (int, []byte) {
				if path == "/api/app-version" {
					b, _ := json.Marshal(version.Info{AgentContractVersion: 19, AgentContractFingerprint: contract.AgentHubV19Fingerprint})
					return 200, b
				}
				if strings.Contains(path, "/modules/proxy") {
					calls.Add(1)
				}
				return 200, []byte(`{"success":true}`)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				status, body := respond(r.Method, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write(body)
			}))
			defer server.Close()
			endpoint := server.URL
			token := "example"
			key := ""
			env := x.request.Instance.EnvironmentID
			if mode == "edge" {
				endpoint = "edge://example"
				c := &contractConn{}
				tunnel := edge.NewAgentTunnelWithConn(env, c)
				tunnel.APIKeyID = key
				tunnel.TokenHash = edge.AgentTokenHash(token)
				tunnel.Capabilities = edge.AdvertisedEdgeCommands()
				c.send = func(message *edge.TunnelMessage) {
					if message.Type != edge.MessageTypeCommandRequest {
						return
					}
					status, body := respond(message.Method, message.Path)
					pending, ok := tunnel.Pending.Load(message.ID)
					require.True(t, ok)
					pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: message.ID, Type: edge.MessageTypeResponse, Status: status, Body: body}
				}
				edge.GetRegistry().Register(env, tunnel)
				defer edge.GetRegistry().Unregister(env)
			}
			require.NoError(t, x.db.Create(&environment.Environment{ID: env, ApiUrl: endpoint, Name: "Example", Enabled: true, IsEdge: mode == "edge", Status: "pending", AccessToken: &token, ApiKeyID: nil}).Error)
			hosts := environment.NewEnvironmentService(x.db, server.Client(), nil, nil, x.settings, nil)
			m := &Module{Settings: x.settings, Modules: x.mods, Hub: &Hub{DB: x.db.DB}, Transport: &HostTransport{Hosts: hosts}}
			require.NoError(t, m.saveInstance(t.Context(), x.request.Instance))
			for _, path := range []string{"/api/environments/0/modules/proxy/resolvers", "/api/environments/0/modules/proxy/certificate-check"} {
				_, finish, err := hosts.AgentAccess(t.Context(), env, &token, "POST", path)
				if finish != nil {
					finish()
				}
				require.ErrorIs(t, err, environment.ErrAgentUpdateRequired)
			}
			err := m.stubInstance(t.Context(), x.request.Instance.ID)
			require.ErrorContains(t, err, "agent.update-required")
			require.Zero(t, calls.Load(), "compatibility must refuse before any new module request")
		})
	}
}
