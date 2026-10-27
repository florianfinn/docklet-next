// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: c477a3d9aba28f8ae7ab6683b56e380a36fa1aa1c6d8e7de3bffeb4b61b7305f
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original escaped route identity regression tests for issue #260.

package bootstrap

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/vulnerability"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type escHTTP func(*http.Request) (*http.Response, error)

func (f escHTTP) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type escAuthzConn struct {
	tunnel *edge.AgentTunnel
	agent  http.Handler
}

func (c *escAuthzConn) Send(msg *edge.TunnelMessage) error {
	if msg.Type != edge.MessageTypeCommandRequest {
		return nil
	}
	if !edge.ValidateEdgeCommand(msg.Command, msg.Method, msg.Path, false) {
		return errors.New("invalid agent command")
	}
	rr := httptest.NewRecorder()
	c.agent.ServeHTTP(rr, httptest.NewRequest(msg.Method, msg.Path, bytes.NewReader(msg.Body)))
	raw, _ := c.tunnel.Pending.Load(msg.ID)
	raw.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: msg.ID, Type: edge.MessageTypeCommandComplete, Status: rr.Code, Body: rr.Body.Bytes(), Headers: map[string]string{"Content-Type": rr.Header().Get("Content-Type")}}
	return nil
}
func (*escAuthzConn) Receive() (*edge.TunnelMessage, error) { return nil, io.EOF }
func (*escAuthzConn) IsExpectedReceiveError(error) bool     { return true }
func (*escAuthzConn) Close() error                          { return nil }
func (*escAuthzConn) IsClosed() bool                        { return false }
func (*escAuthzConn) Transport() string                     { return "test" }
func TestEscapedImageRouteRequiresImagePermission(t *testing.T) {
	metaRouter := newEchoInternal()
	meta := humaecho.NewWithGroup(metaRouter, metaRouter.Group("/api"), huma.DefaultConfig("meta", "test"))
	var images *image.Module
	images.RegisterRoutes(meta, handlerutil.NewActivityAppContext(t.Context()))
	vmetaRouter := newEchoInternal()
	vmeta := humaecho.NewWithGroup(vmetaRouter, vmetaRouter.Group("/api"), huma.DefaultConfig("vmeta", "test"))
	vulnerability.New(nil).RegisterRoutes(vmeta, handlerutil.NewActivityAppContext(t.Context()))
	matcher := authz.NewPermissionMatcher()
	matcher.CollectFromHumaAPI(meta)
	matcher.CollectFromHumaAPI(vmeta)
	actual, _ := matcher.Lookup("GET", "/images/team%2Fvulnerabilities").Get()
	require.Equal(t, authz.PermImagesRead, actual)
	decoded, _ := matcher.Lookup("GET", "/images/team/vulnerabilities").Get()
	require.Equal(t, authz.PermVulnsRead, decoded)
	for _, edgeMode := range []bool{false, true} {
		for _, grant := range []string{authz.PermVulnsRead, authz.PermImagesRead} {
			t.Run(fmt.Sprint(edgeMode, "/", grant), func(t *testing.T) {
				ps := authz.NewPermissionSet()
				ps.AddEnv("example", grant)
				agent := newEchoInternal()
				agentAPI := humaecho.NewWithGroup(agent, agent.Group("/api"), huma.DefaultConfig("agent", "test"))
				agentAPI.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
				})
				type input struct {
					ID      string `path:"id"`
					ImageID string `path:"imageId"`
				}
				type output struct {
					Body struct {
						ImageID string `json:"imageId"`
					}
				}
				middleware.RegisterWithPermission(agentAPI, huma.Operation{OperationID: "inspect", Method: "GET", Path: "/environments/{id}/images/{imageId}"}, authz.PermImagesRead, func(_ context.Context, in *input) (*output, error) {
					out := &output{}
					out.Body.ImageID = in.ImageID
					return out, nil
				})
				registry := edge.NewTunnelRegistry()
				if edgeMode {
					conn := &escAuthzConn{agent: agent}
					tunnel := edge.NewAgentTunnelWithConn("example", conn)
					conn.tunnel = tunnel
					registry.Register("example", tunnel)
				}
				security := middleware.EnvironmentProxySecurity{VulnerabilityEnabled: func(context.Context) bool { return false }, HTTPClient: &http.Client{Transport: escHTTP(func(r *http.Request) (*http.Response, error) {
					rr := httptest.NewRecorder()
					agent.ServeHTTP(rr, r)
					return rr.Result(), nil
				})}}
				hub := newEchoInternal()
				g := hub.Group("/api")
				g.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) {
					if edgeMode {
						return "edge://example", nil, true, nil
					}
					return "https://agent.example.com", nil, true, nil
				}, func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) { return ps, nil, true }, matcher, registry, nil, security))
				g.Any("/environments/:id/*", func(c *echo.Context) error { return c.NoContent(418) })
				rr := httptest.NewRecorder()
				hub.ServeHTTP(rr, httptest.NewRequest("GET", "/api/environments/example/images/team%2Fvulnerabilities", nil))
				if grant == authz.PermVulnsRead {
					require.Equal(t, 403, rr.Code, "actual image route requires images:read")
				} else {
					require.Equal(t, 200, rr.Code, "image permission must admit image route")
					var out output
					require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out.Body))
					require.Equal(t, "team/vulnerabilities", out.Body.ImageID)
				}
			})
		}
	}
}
