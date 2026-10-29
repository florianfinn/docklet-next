// Source: docklet-next/issues/292
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original route test for proxied domain requests through the environment service.

package environment

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
)

type proxiedRouteInput struct {
	ID string `path:"id"`
}

type proxiedRouteOutput struct {
	Body map[string]any
}

// TestProxiedDomainRouteAgentEnvelope drives a domain route through the real
// ProxyJSONRequest so outdated, refused and unreachable hosts answer like the middleware.
func TestProxiedDomainRouteAgentEnvelope(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, state := range []string{"update-required", "invalid-token", "unavailable", "disconnected", "domain-404"} {
			t.Run(mode+"/"+state, func(t *testing.T) {
				info := version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}
				if state == "update-required" {
					info.AgentContractFingerprint = "different"
				}
				f := newAgentFixture(t, mode, info)
				if state == "disconnected" {
					if mode == "direct" {
						t.Skip("Direct hosts have no tunnel")
					}
					edge.GetRegistry().Unregister(f.id)
				}
				reply := func(path string) (int, string, bool) {
					switch {
					case state == "unavailable":
						return 0, "", false
					case state == "invalid-token":
						return http.StatusUnauthorized, `{"error":"synthetic-private.example.com"}`, true
					case state == "domain-404" && path == "/api/environments/0/settings/public":
						return http.StatusNotFound, `{"error":"setting not found"}`, true
					}
					return http.StatusOK, string(f.respond(http.MethodGet, path, nil)), true
				}
				if mode == "direct" {
					f.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
						status, body, ok := reply(r.URL.Path)
						if !ok {
							return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("synthetic-private.example.com refused")}
						}
						return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
					})
				} else {
					f.conn.send = func(m *edge.TunnelMessage) {
						if m.Type != edge.MessageTypeCommandRequest {
							return
						}
						status, body, ok := reply(m.Path)
						if !ok {
							return
						}
						pending, found := f.tunnel.Pending.Load(m.ID)
						require.True(t, found)
						pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: m.ID, Type: edge.MessageTypeResponse, Status: status, Body: []byte(body)}
					}
				}

				_, api := humatest.New(t)
				huma.Get(api, "/environments/{id}/settings/public", func(ctx context.Context, in *proxiedRouteInput) (*proxiedRouteOutput, error) {
					if mode == "edge" && state == "unavailable" {
						var cancel context.CancelFunc
						ctx, cancel = context.WithTimeout(ctx, 500*time.Millisecond)
						defer cancel()
					}
					out, err := handlerutil.RemoteJSONProxy(f.service.ProxyJSONRequest).JSON[map[string]any](ctx, in.ID, http.MethodGet, "/api/environments/0/settings/public", nil)
					if err != nil {
						return nil, err
					}
					return &proxiedRouteOutput{Body: *out}, nil
				})
				resp := api.Get("/environments/" + f.id + "/settings/public")
				body := resp.Body.String()
				require.NotContains(t, body, "synthetic-private")
				if state == "domain-404" {
					require.Equal(t, http.StatusNotFound, resp.Code)
					require.Contains(t, body, "setting not found")
					require.NotContains(t, body, `"code":"agent.`)
					return
				}
				expected := http.StatusBadGateway
				code := state
				if state == "update-required" {
					expected = http.StatusConflict
				}
				if state == "disconnected" {
					code = "unavailable"
				}
				require.Equal(t, expected, resp.Code)
				require.JSONEq(t, `{"success":false,"code":"agent.`+code+`"}`, body)
			})
		}
	}
}
