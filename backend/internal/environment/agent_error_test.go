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
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAgentAccessFailureCodes(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, state := range []string{"update-required", "invalid-token", "unavailable", "stale-token"} {
			t.Run(mode+"/"+state, func(t *testing.T) {
				f := newAgentFixture(t, mode, version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: "different"})
				ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
				defer cancel()
				if mode == "direct" && state != "update-required" {
					f.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
						if state == "unavailable" {
							return nil, errors.New("agent unreachable")
						}
						return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"invalid token"}`)), Request: r}, nil
					})
				}
				if mode == "edge" && state != "update-required" {
					f.conn.send = func(m *edge.TunnelMessage) {
						if state == "unavailable" {
							return
						}
						pending, ok := f.tunnel.Pending.Load(m.ID)
						require.True(t, ok)
						pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: m.ID, Type: edge.MessageTypeResponse, Status: http.StatusUnauthorized, Body: []byte(`{"error":"invalid token"}`)}
					}
				}
				resolver := func(context.Context, string) (string, *string, bool, error) {
					env, err := f.service.GetEnvironmentByID(ctx, f.id)
					require.NoError(t, err)
					if state == "stale-token" {
						env.AccessToken = new("test-only")
					}
					return env.ApiUrl, env.AccessToken, env.Enabled, nil
				}
				router := echo.New()
				router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", resolver, nil, nil, edge.GetRegistry(), nil, middleware.EnvironmentProxySecurity{AgentAccess: f.service.AgentAccess, HTTPClient: f.service.httpClient}))
				router.GET("/api/environments/:id/containers", func(c *echo.Context) error { return c.NoContent(500) })
				rec := httptest.NewRecorder()
				requestCtx := ctx
				if mode == "edge" && state == "unavailable" {
					var finish context.CancelFunc
					requestCtx, finish = context.WithTimeout(ctx, 500*time.Millisecond)
					defer finish()
				}
				router.ServeHTTP(rec, httptest.NewRequest("GET", "/api/environments/"+f.id+"/containers", nil).WithContext(requestCtx))
				code := state
				if state == "stale-token" {
					code = "invalid-token"
				}
				require.Contains(t, rec.Body.String(), `"code":"agent.`+code+`"`)
				expectedStatus := http.StatusBadGateway
				if state == "update-required" {
					expectedStatus = http.StatusConflict
				}
				require.Equal(t, expectedStatus, rec.Code)
				require.Zero(t, f.count("GET /api/environments/0/containers"))
			})
		}
	}
}

func TestUpdateRequiredHealthOutcome(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			f := newAgentFixture(t, mode, version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: "different"})
			gate := newAdmissionGateForEnvironmentTestInternal(t)
			require.NoError(t, f.service.SetScheduler(t.Context(), &environmentTestSchedulerInternal{}, gate))
			outcome, err := f.service.runHealthCheckInternal(t.Context(), f.id)
			require.ErrorIs(t, err, ErrAgentUpdateRequired)
			require.Equal(t, scheduler.Partial, outcome.Status)
			require.Equal(t, "Agent update required", outcome.Message)
			require.NotContains(t, outcome.Message, "offline")
			require.Zero(t, f.count("POST /api/container-registries/sync"))
		})
	}
}

func TestAgentForwardingFailureCodesAfterCompatibility(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, 0} {
			t.Run(mode+"/"+http.StatusText(status), func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				f := newAgentFixture(t, mode, version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
				_, err := f.service.CheckAgentCompatibility(ctx, f.id)
				require.NoError(t, err)
				if mode == "direct" {
					f.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
						if status == 0 {
							// Cancel only after the forwarded request reaches the transport.
							cancel()
							return nil, errors.New("agent unreachable")
						}
						return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("agent rejected credential")), Request: r}, nil
					})
				} else {
					f.conn.send = func(m *edge.TunnelMessage) {
						if status == 0 {
							// Cancel only after the forwarded request reaches the transport.
							cancel()
							return
						}
						pending, ok := f.tunnel.Pending.Load(m.ID)
						require.True(t, ok)
						pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: m.ID, Type: edge.MessageTypeResponse, Status: status, Body: []byte("agent rejected credential")}
					}
				}
				resolver := func(context.Context, string) (string, *string, bool, error) {
					env, err := f.service.GetEnvironmentByID(ctx, f.id)
					require.NoError(t, err)
					return env.ApiUrl, env.AccessToken, true, nil
				}
				router := echo.New()
				router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", resolver, nil, nil, edge.GetRegistry(), nil, middleware.EnvironmentProxySecurity{AgentAccess: f.service.AgentAccess, HTTPClient: f.service.httpClient}))
				router.GET("/api/environments/:id/containers", func(c *echo.Context) error { return c.NoContent(500) })
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest("GET", "/api/environments/"+f.id+"/containers", nil).WithContext(ctx))
				if status == http.StatusForbidden {
					require.Equal(t, http.StatusForbidden, rec.Code)
					require.Equal(t, "agent rejected credential", rec.Body.String(), "authorization responses retain their original meaning")
					require.NotContains(t, rec.Body.String(), "agent.invalid-token")
					return
				}
				code := "invalid-token"
				if status == 0 {
					code = "unavailable"
				}
				require.Contains(t, rec.Body.String(), `"code":"agent.`+code+`"`)
				if status != 0 {
					require.Equal(t, http.StatusBadGateway, rec.Code)
				} else {
					require.Contains(t, []int{http.StatusBadGateway, http.StatusGatewayTimeout}, rec.Code)
				}
				require.Equal(t, 1, f.count("GET /api/app-version"), "the error is classified on forwarding after a cached verdict")
			})
		}
	}
}
