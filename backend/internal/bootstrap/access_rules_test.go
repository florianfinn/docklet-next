// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package bootstrap

import (
	"context"
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestM15RuleCompleteness(t *testing.T) {
	hub := newEchoInternal()
	cfg := &config.Config{AppUrl: "https://hub.example.com", Environment: config.AppEnvironmentProduction}
	registered := api.SetupAPI(hub, hub.Group("/api"), handlerutil.NewActivityAppContext(t.Context()), cfg, api.HandlerDeps{})
	raw, err := json.Marshal(registered.OpenAPI())
	require.NoError(t, err)
	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &spec))
	want := map[string]bool{}
	humaRoutes := map[string]bool{}
	for path, item := range spec.Paths {
		for method, op := range item {
			if op.OperationID == "" {
				continue
			}
			want[op.OperationID] = true
			parts := strings.Split(path, "/")
			for i, s := range parts {
				if strings.HasPrefix(s, "{") {
					parts[i] = ":" + strings.Trim(s, "{}")
				}
			}
			humaRoutes[strings.ToUpper(method)+" /api"+strings.Join(parts, "/")] = true
		}
	}
	router, _ := newRouter(RouterParams{Context: t.Context(), Lifecycle: fxtest.NewLifecycle(t), Config: cfg, HandlerDeps: api.HandlerDeps{Environment: environment.New(&environment.EnvironmentService{}, nil, nil, nil, cfg, nil), Project: project.New(&project.ProjectService{}, nil), Container: container.New(&container.ContainerService{}, nil, nil, nil), System: system.New(&system.SystemService{}, nil, nil, nil, nil)}, AuthMiddleware: auth.NewAuthMiddleware(nil, cfg), TunnelRegistry: edge.NewTunnelRegistry()})
	for _, r := range router.Router().Routes() {
		key := r.Method + " " + r.Path
		if strings.HasPrefix(r.Path, "/api") && !humaRoutes[key] {
			want[key] = true
		}
	}
	for key := range want {
		_, ok := access.Rules[key]
		assert.True(t, ok, "missing rule: %s", key)
	}
	for key := range access.Rules {
		assert.True(t, want[key], "stale rule: %s", key)
	}
}

func TestAllAdministratorOperationsDenyUserBeforeHandler(t *testing.T) {
	router := newEchoInternal()
	ps := access.Synthesize("viewer", nil)
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.SetRequest(c.Request().WithContext(context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, ps)))
			return next(c)
		}
	})
	cfg := &config.Config{Environment: config.AppEnvironmentProduction}
	registered := api.SetupAPI(router, router.Group("/api"), handlerutil.NewActivityAppContext(t.Context()), cfg, api.HandlerDeps{})
	count := 0
	for path, item := range registered.OpenAPI().Paths {
		for method, op := range map[string]*huma.Operation{"GET": item.Get, "POST": item.Post, "PUT": item.Put, "DELETE": item.Delete, "PATCH": item.Patch, "HEAD": item.Head, "OPTIONS": item.Options} {
			if op == nil || access.Rules[op.OperationID].Kind != access.Admin {
				continue
			}
			count++
			t.Run(op.OperationID, func(t *testing.T) {
				parts := strings.Split(path, "/")
				for i, s := range parts {
					if strings.HasPrefix(s, "{") {
						parts[i] = "0"
					}
				}
				r := httptest.NewRequest(method, "/api"+strings.Join(parts, "/"), http.NoBody)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				require.Equal(t, 403, w.Code, w.Body.String())
				if method != "HEAD" {
					require.Contains(t, w.Body.String(), "access.admin-required")
				}
			})
		}
	}
	require.Greater(t, count, 100)
}

func TestUserSupportingGrantsCoverAllowedHandlerContracts(t *testing.T) {
	spec := api.SetupAPIForSpec().OpenAPI()
	for _, path := range spec.Paths {
		for _, op := range []*huma.Operation{path.Get, path.Post, path.Put, path.Delete, path.Patch} {
			if op == nil {
				continue
			}
			rule := access.Rules[op.OperationID]
			if rule.Kind != access.App {
				continue
			}
			permission, _ := op.Metadata[authz.MetaRequiredPermission].(string)
			if permission == "" {
				continue
			}
			ps := access.Synthesize("viewer", []userDTO.AppAssignment{{EnvironmentID: "0", ProjectID: "project-uuid", Level: string(rule.Level)}})
			require.True(t, ps.Allows(permission, "0"), "%s requires %s", op.OperationID, permission)
		}
	}
}
