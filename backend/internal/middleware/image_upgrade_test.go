// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original image and registry safety contract for issue #229.

package middleware

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/config"
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

func TestPR1UpgradeCapBeforeDirectOrEdgeDispatch(t *testing.T) {
	old := config.Version
	config.Version = "2.0.0"
	defer func() { config.Version = old }()
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(204) }))
			defer agent.Close()
			target := agent.URL
			if mode == "edge" {
				target = "edge://example-host"
			}
			router := echo.New()
			router.Use(NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) {
				return target, new("manager-token"), true, nil
			}, func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) {
				return authz.SudoPermissionSet(), &user.Actor{ID: "operator"}, true
			}, nil, nil, nil))
			router.Any("/api/environments/:id/*", func(c *echo.Context) error { return c.NoContent(500) })
			for _, tc := range []struct {
				body, code string
				status     int
			}{{`{"targetVersion":"2.1.0"}`, "update.requires-hub-update", 409}, {`{}`, "update.invalid-target", 400}, {`{"targetVersion":"unknown"}`, "update.system-version-unknown", 503}} {
				req := httptest.NewRequest(http.MethodPost, "/api/environments/example-host/system/upgrade", strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				assert.Equal(t, tc.status, rec.Code, rec.Body.String())
				assert.Contains(t, rec.Body.String(), tc.code)
			}
			require.Zero(t, calls.Load(), "hub cap must refuse before remote effects")
		})
	}
}
