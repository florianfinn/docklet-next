// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original hub history access through real Huma and environment middleware.

package bootstrap

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/history"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	authDTO "github.com/florianfinn/docklet-next/types/auth"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/contract"
	dto "github.com/florianfinn/docklet-next/types/history"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHB14HistoryRightsAndHubServing(t *testing.T) {
	f := newSignInRoutingFixture(t)
	var forwarded atomic.Int32
	sample := dto.Sample{Host: dto.HostSample{MemoryTotal: 100, DiskTotal: 100, DiskSource: "/example"}, Containers: []dto.ContainerSample{{ID: "app-id", Name: "app", ProjectID: "assigned", State: "exited", ExitCode: 1}, {ID: "other-id", Name: "private", ProjectID: "other", State: "exited", ExitCode: 1}}, Projects: []dto.ProjectSample{{ID: "assigned", Name: "Assigned"}, {ID: "other", Name: "Other"}}}
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "HEAD" && r.URL.Path == "/api/environments/0/system/health":
			w.WriteHeader(204)
		case r.URL.Path == "/api/app-version":
			_ = json.NewEncoder(w).Encode(version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
		case r.URL.Path == dto.SamplePath:
			_ = json.NewEncoder(w).Encode(base.ApiResponse[dto.Sample]{Success: true, Data: sample})
		case strings.HasSuffix(r.URL.Path, "/projects/assigned/runtime"):
			_ = json.NewEncoder(w).Encode(base.ApiResponse[projectDTO.Details]{Success: true, Data: projectDTO.Details{ID: "assigned", RuntimeServices: []projectDTO.RuntimeService{{ContainerID: "app-id", ContainerName: "app"}}}})
		default:
			forwarded.Add(1)
			http.Error(w, "history must stay on hub", 599)
		}
	}))
	defer agent.Close()
	credential := uuid.NewString()
	require.NoError(t, f.db.Model(&environment.Environment{}).Where("id='example'").Updates(map[string]any{"api_url": agent.URL, "access_token": credential}).Error)
	require.NoError(t, f.db.Create(&environment.Environment{ID: "other-host", Name: "Other host", ApiUrl: agent.URL, Enabled: true, AccessToken: &credential}).Error)
	hosts := environment.NewEnvironmentService(f.db, agent.Client(), nil, nil, nil, f.keys)
	service := history.NewService(f.db, nil, hosts, nil, nil)
	host, err := hosts.GetEnvironmentByID(t.Context(), "example")
	require.NoError(t, err)
	require.NoError(t, service.SampleHost(t.Context(), *host, time.Now().UTC()))
	viewer, err := f.users.CreateUser(t.Context(), &user.User{Username: "viewer"})
	require.NoError(t, err)
	require.NoError(t, f.db.Create(&access.Assignment{ID: "assignment", UserID: viewer.ID, EnvironmentID: "example", ProjectID: "assigned", Level: "restricted", CreatedAt: time.Now().UTC()}).Error)
	tokens, err := f.svc.CompleteLogin(t.Context(), viewer, authDTO.SessionMeta{}, "local")
	require.NoError(t, err)
	deps := api.HandlerDeps{Auth: auth.New(f.svc, f.users, nil), ApiKey: apikey.New(f.keys), Role: role.New(f.roles), Environment: environment.New(hosts, nil, f.keys, nil, f.cfg, nil), History: history.New(service, nil)}
	router := echo.New()
	controller := resourceAccessController(deps)
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.SetRequest(c.Request().WithContext(access.WithController(c.Request().Context(), controller)))
			return next(c)
		}
	})
	matcher := authz.NewPermissionMatcher()
	matcher.CollectFromHumaAPI(api.SetupAPIForSpec())
	router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(_ context.Context, id string) (string, *string, bool, error) {
		h, err := hosts.GetEnvironmentByID(t.Context(), id)
		if err != nil {
			return "", nil, false, err
		}
		return h.ApiUrl, h.AccessToken, h.Enabled, nil
	}, createAuthValidatorInternal(deps), matcher, edge.NewTunnelRegistry(), nil))
	api.SetupAPI(router, router.Group("/api"), handlerutil.NewActivityAppContext(t.Context()), f.cfg, deps)
	read := func(path, token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest("GET", "/api"+path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	for _, tc := range []struct {
		path   string
		status int
	}{{"/environments/example/containers/app/history", 200}, {"/environments/other-host/containers/app/history", 404}, {"/environments/example/projects/assigned/history", 200}, {"/environments/example/projects/other/history", 404}, {"/environments/example/history/host", 403}, {"/history/capacity", 403}} {
		r := read(tc.path, tokens.AccessToken)
		require.Equal(t, tc.status, r.Code, r.Body.String())
	}
	conditions := read("/environments/example/runtime-conditions", tokens.AccessToken)
	require.Equal(t, 200, conditions.Code, conditions.Body.String())
	require.Contains(t, conditions.Body.String(), `"name":"app"`)
	require.NotContains(t, conditions.Body.String(), `"name":"private"`)
	administrator := read("/environments/example/runtime-conditions", f.tokens.AccessToken)
	require.Equal(t, 200, administrator.Code)
	require.Contains(t, administrator.Body.String(), `"name":"private"`)
	require.EqualValues(t, 0, forwarded.Load())
	// Removal uses the real service transaction and its ephemeral invalidator.
	hosts.SetHistoryInvalidator(service.Forget)
	require.NoError(t, hosts.DeleteEnvironment(t.Context(), "example", nil, nil))
	var count int64
	require.NoError(t, f.db.Table("history_buckets").Where("host_id='example'").Count(&count).Error)
	require.Zero(t, count)
	_, err = service.Conditions(t.Context(), "example")
	require.Error(t, err)
}
