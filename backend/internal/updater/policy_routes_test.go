// Source: docklet-next/docs/design/update-policies.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Actual HTTP policy boundary refuses standalone actions and missing grants.

package updater

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPolicyHTTPStackOnlyAndGrantConjunction(t *testing.T) {
	f := newPolicyFixture(t)
	router := echo.New()
	api := humaecho.New(router, huma.DefaultConfig("test", "test"))
	registerImagePolicyRoutes(api, f.s)
	for _, test := range []struct {
		kind   string
		grants *authz.PermissionSet
		status int
	}{
		{"container", &authz.PermissionSet{Sudo: true}, 400},
		{"project", &authz.PermissionSet{Global: map[string]struct{}{authz.PermJobsManage: {}}}, 403},
		{"project", &authz.PermissionSet{Sudo: true}, 200},
	} {
		req := httptest.NewRequest(http.MethodPut, "/environments/0/updater/policies/"+test.kind+"/example-stack", strings.NewReader(`{"mode":"apply","expectedRevision":0}`))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), middleware.ContextKeyUserPermissions, test.grants)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "example-actor")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req.WithContext(ctx))
		require.Equal(t, test.status, rec.Code, rec.Body.String())
	}
	var count int64
	require.NoError(t, f.db.Model(&policy.Policy{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.Zero(t, f.admissions)
	require.Zero(t, f.checks)
	require.Zero(t, f.notifications)
}

func TestSystemImageHintHTTPUsesApprovedPathAndReadGrant(t *testing.T) {
	f := newPolicyFixture(t)
	calls := 0
	f.p.ports.hints = func(context.Context, string) (dto.SystemImageHints, error) {
		calls++
		return dto.SystemImageHints{HubVersion: "2.0.0", Images: []dto.SystemImageHint{}}, nil
	}
	router := echo.New()
	api := humaecho.New(router, huma.DefaultConfig("test", "test"))
	registerSystemImageHintRoutes(api, f.s)
	for _, test := range []struct {
		permission string
		status     int
	}{{authz.PermSystemRead, 403}, {authz.PermImageUpdatesRead, 200}} {
		req := httptest.NewRequest(http.MethodGet, "/environments/0/updater/system-image-hints", http.NoBody)
		ctx := context.WithValue(req.Context(), middleware.ContextKeyUserPermissions, &authz.PermissionSet{Global: map[string]struct{}{test.permission: {}}})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req.WithContext(ctx))
		require.Equal(t, test.status, response.Code, response.Body.String())
	}
	require.Equal(t, 1, calls)
	require.Zero(t, f.admissions)
	require.Zero(t, f.checks)
}

func TestPolicyHTTPNotifyOffNeedOnlyReadAndCheck(t *testing.T) {
	for _, mode := range []string{"off", "notify", "apply"} {
		t.Run(mode, func(t *testing.T) {
			f := newPolicyFixture(t)
			router := echo.New()
			api := humaecho.New(router, huma.DefaultConfig("test", "test"))
			registerImagePolicyRoutes(api, f.s)
			grants := &authz.PermissionSet{Global: map[string]struct{}{authz.PermProjectsRead: {}, authz.PermImageUpdatesRead: {}, authz.PermImageUpdatesCheck: {}}}
			req := httptest.NewRequest(http.MethodPut, "/environments/0/updater/policies/project/stack", strings.NewReader(`{"mode":"`+mode+`","expectedRevision":0}`))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), middleware.ContextKeyUserPermissions, grants)
			ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "example-actor")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req.WithContext(ctx))
			expected := 200
			if mode == "apply" {
				expected = 403
			}
			require.Equal(t, expected, response.Code, response.Body.String())
		})
	}
}
