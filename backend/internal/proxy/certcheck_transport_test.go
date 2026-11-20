// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestP8ManualCertificateDirectEdgeIngress(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			x := newFixture(t)
			x.a.Gate = x.mods
			x.request.Instance.Authority = dto.FileAuthority
			require.NoError(t, x.db.Exec("INSERT INTO api_keys(id,name,key_hash,key_prefix,kind) VALUES(?,?,?,?,?)", "example", "Example key", "example", "example", "environment").Error)
			calls := 0
			observation := dto.CertificateObservation{ObservedAt: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), InstanceID: x.request.Instance.ID, Epoch: dto.CertificateEpoch{ContainerID: "agent-local-container", StartedAt: initialEpoch}, Route: dto.CertificateRoute{Name: "app", Host: "app.example.test", SNI: "app.example.test"}, Peer: dto.CertificatePeer{Subject: "CN=agent-local.example.test", Issuer: "CN=Example CA", SANs: []string{"app.example.test"}, NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), NotAfter: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), SHA256: strings.Repeat("b", 64)}, HostnameMatch: true, Validity: dto.CertificateValid}
			x.a.Engines[dto.Traefik] = &probeObserveEngine{Engine: x.e, check: func(_ context.Context, _ dto.Instance, q dto.CertificateCheckRequest) (dto.CertificateObservation, error) {
				calls++
				require.Equal(t, "app.example.test", q.Host)
				return observation, nil
			}}
			admin := true
			routerFor := func(m *Module, agent bool) *echo.Echo {
				router := echo.New()
				api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("certificate", "test"))
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, &authz.PermissionSet{Sudo: admin})))
				})
				m.RegisterRoutes(api, agent)
				return router
			}
			host := routerFor(&Module{Agent: x.a, Settings: x.settings, Modules: x.mods}, true)
			serve := func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/app-version" {
					w.Header().Set("Content-Type", "application/json")
					require.NoError(t, json.NewEncoder(w).Encode(version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}))
					return
				}
				if r.URL.Path == "/api/environments/0/system/health" {
					w.WriteHeader(200)
					return
				}
				host.ServeHTTP(w, r)
			}
			server := httptest.NewServer(http.HandlerFunc(serve))
			defer server.Close()
			env := x.request.Instance.EnvironmentID
			endpoint := server.URL
			token := "example"
			if mode == "edge" {
				endpoint = "edge://example"
				conn := &contractConn{}
				tunnel := edge.NewAgentTunnelWithConn(env, conn)
				tunnel.APIKeyID = "example"
				tunnel.TokenHash = edge.AgentTokenHash(token)
				tunnel.Capabilities = edge.AdvertisedEdgeCommands()
				conn.send = func(message *edge.TunnelMessage) {
					if message.Type != edge.MessageTypeCommandRequest {
						return
					}
					if strings.HasSuffix(message.Path, "certificate-check") {
						require.Equal(t, "proxy.certificate.check", message.Command)
						require.True(t, edge.ValidateEdgeCommand(message.Command, message.Method, message.Path, false))
					}
					request := httptest.NewRequest(message.Method, message.Path, bytes.NewReader(message.Body))
					request.Header.Set("Content-Type", "application/json")
					response := httptest.NewRecorder()
					serve(response, request)
					pending, ok := tunnel.Pending.Load(message.ID)
					require.True(t, ok)
					pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: message.ID, Type: edge.MessageTypeResponse, Status: response.Code, Body: response.Body.Bytes()}
				}
				edge.GetRegistry().Register(env, tunnel)
				defer edge.GetRegistry().Unregister(env)
			}
			require.NoError(t, x.db.Create(&environment.Environment{ID: env, ApiUrl: endpoint, Name: "Example", Enabled: true, IsEdge: mode == "edge", Status: "pending", AccessToken: &token, ApiKeyID: new("example")}).Error)
			hosts := environment.NewEnvironmentService(x.db, server.Client(), nil, nil, x.settings, nil)
			m := &Module{Settings: x.settings, Modules: x.mods, Hub: &Hub{DB: x.db.DB}, Transport: &HostTransport{Hosts: hosts}}
			require.NoError(t, m.saveInstance(t.Context(), x.request.Instance))
			hub := routerFor(m, false)
			q := dto.CertificateCheckRequest{File: "routes/app.yml", Router: "app", SourceHash: strings.Repeat("a", 64), Host: "app.example.test"}
			response := postContract(hub, "/api/modules/proxy/instances/instance-one/certificate-check", q)
			require.Equal(t, 200, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), "agent-local.example.test")
			require.Equal(t, 1, calls)
			admin = false
			response = postContract(hub, "/api/modules/proxy/instances/instance-one/certificate-check", q)
			require.Equal(t, 403, response.Code, response.Body.String())
			require.Equal(t, 1, calls)
			response = postContract(host, "/api/environments/0/modules/proxy/certificate-check", dto.HostCertificateCheckRequest{Instance: x.request.Instance, CertificateCheckRequest: q})
			require.Equal(t, 403, response.Code, response.Body.String())
			require.Equal(t, 1, calls)
			admin = true
			require.NoError(t, x.settings.UpdateSetting(t.Context(), features.ProxySettingKey, "false"))
			response = postContract(hub, "/api/modules/proxy/instances/instance-one/certificate-check", q)
			require.Equal(t, 404, response.Code, response.Body.String())
			response = postContract(host, "/api/environments/0/modules/proxy/certificate-check", dto.HostCertificateCheckRequest{Instance: x.request.Instance, CertificateCheckRequest: q})
			require.Equal(t, 403, response.Code, response.Body.String())
			require.Equal(t, 1, calls)
			for _, table := range []string{"proxy_journal", "proxy_operations", "proxy_reservations"} {
				var n int64
				require.NoError(t, x.db.Table(table).Count(&n).Error)
				require.Zero(t, n)
			}
		})
	}
}
