// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original image and registry safety contract for issue #229.

package middleware

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPR1InlineCredentialAlternativesHaveZeroEffects(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			var effects atomic.Int32
			router := echo.New()
			if mode == "local" {
				api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("test", "test"))
				api.UseMiddleware(RejectInlineImageCredentials(api))
				huma.Register(api, huma.Operation{OperationID: "pull", Method: "POST", Path: "/environments/{id}/images/pull"}, func(ctx context.Context, in *struct {
					ID string `path:"id"`
				}) (*struct{ Body string }, error) {
					effects.Add(1)
					return &struct{ Body string }{"done"}, nil
				})
			} else {
				remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { effects.Add(1); w.WriteHeader(204) }))
				defer remote.Close()
				target := remote.URL
				if mode == "edge" {
					target = "edge://example-host"
				}
				router.Use(NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) {
					return target, new("manager-token"), true, nil
				}, func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) {
					return authz.SudoPermissionSet(), &user.Actor{ID: "operator"}, true
				}, nil, nil, nil))
				router.Any("/api/environments/:id/*", func(c *echo.Context) error { effects.Add(1); return c.NoContent(204) })
			}
			for _, tc := range []struct{ query, header string }{{"?credentials=", ""}, {"?registry-auth=synthetic", ""}, {"", "X-Registry-Auth"}, {"", "X-Registry-Config"}} {
				request := httptest.NewRequest("POST", "/api/environments/example-host/images/pull"+tc.query, strings.NewReader(`{}`))
				if tc.header != "" {
					request.Header[tc.header] = []string{""}
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				assert.Equal(t, 400, response.Code)
				assert.Contains(t, response.Body.String(), "registry.inline-credentials-not-allowed")
			}
			require.Zero(t, effects.Load())
		})
	}
}
