// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original authenticated Huma/environment K-3 and legacy surface proofs.

package bootstrap

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	authDTO "github.com/florianfinn/docklet-next/types/auth"
	"github.com/florianfinn/docklet-next/types/meta"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestScheduledPolicyAdministratorOnlyThroughAuthentication(t *testing.T) {
	f := newSignInRoutingFixture(t)
	viewer, e := f.users.CreateUser(t.Context(), &user.User{Username: "full-user"})
	require.NoError(t, e)
	require.NoError(t, f.users.SetAppAssignments(t.Context(), viewer.ID, []userDTO.AppAssignment{{EnvironmentID: "0", ProjectID: "stack-id", Level: "full"}}))
	tokens, e := f.svc.CompleteLogin(t.Context(), viewer, authDTO.SessionMeta{}, "local")
	require.NoError(t, e)
	router := echo.New()
	group := router.Group("/api")
	deps := api.HandlerDeps{Auth: auth.New(f.svc, nil, nil), ApiKey: apikey.New(f.keys), Role: role.New(f.roles)}
	group.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) {
		t.Error("policy forwarded")
		return "", nil, false, nil
	}, createAuthValidatorInternal(deps), nil, nil, nil))
	config := huma.DefaultConfig("policy-test", "v14")
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{"BearerAuth": {Type: "http", Scheme: "bearer"}}
	api := humaecho.NewWithGroup(router, group, config)
	api.UseMiddleware(auth.NewHumaMiddleware(api, f.svc, f.keys, f.roles, nil, f.cfg))
	api.UseMiddleware(f.svc.ViewerMiddleware(api))
	api.UseMiddleware(middleware.AccessMiddleware(api))
	type input struct {
		ID     string `path:"id"`
		Kind   string `path:"kind"`
		Target string `path:"targetId"`
		Body   struct {
			Mode string `json:"mode"`
		}
	}
	type output struct{ Body map[string]bool }
	calls := 0
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "set-auto-update-policy", Method: http.MethodPut, Path: "/environments/{id}/updater/policies/{kind}/{targetId}", Security: handlerutil.DefaultOperationSecurity()}, authz.PermImageUpdatesCheck, func(context.Context, *input) (*output, error) {
		calls++
		return &output{Body: map[string]bool{"saved": true}}, nil
	})
	for _, tc := range []struct {
		token, mode string
		status      int
	}{{tokens.AccessToken, "scheduled", 403}, {tokens.AccessToken, "check", 200}, {f.tokens.AccessToken, "scheduled", 200}} {
		r := httptest.NewRequest(http.MethodPut, "/api/environments/0/updater/policies/stack/stack-id", strings.NewReader(`{"mode":"`+tc.mode+`"}`))
		r.Header.Set("Authorization", "Bearer "+tc.token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		require.Equal(t, tc.status, w.Code, w.Body.String())
		if tc.status == 403 {
			require.Contains(t, w.Body.String(), `"code":"update.permission-denied"`)
		}
	}
	require.Equal(t, 2, calls)
}

func TestNoLegacyUpdateJobs(t *testing.T) {
	for _, name := range []string{"auto-update", "image-polling"} {
		_, ok := meta.GetJobMetadata(name)
		require.False(t, ok, name)
	}
	paths := api.SetupAPIForSpec().OpenAPI().Paths
	require.Nil(t, paths["/environments/{id}/updater/schedules"])
	require.NotNil(t, paths["/environments/{id}/updater/policies"])
	require.NotNil(t, paths["/updater/check-all"])
	for _, name := range []string{"auto_update_job.go", "image_update_watcher.go"} {
		_, e := os.Stat("../../pkg/scheduler/" + name)
		require.True(t, os.IsNotExist(e))
	}
	source, e := os.ReadFile("jobs_bootstrap.go")
	require.NoError(t, e)
	require.NotContains(t, string(source), "NewAutoUpdateJob")
	require.NotContains(t, string(source), "NewImageUpdateWatcherJob")
	require.NotContains(t, string(source), `"auto-update"`)
	require.NotContains(t, string(source), `"image-polling"`)
	require.NotContains(t, string(source), `"image-policy:"`)
}
