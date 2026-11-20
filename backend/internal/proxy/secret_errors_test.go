// Source: docklet-next/issues/492
// Revision: a00b3f23b9cb0eaa0fa2ca0cb58cbfaa397f1a2b
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 proof that resolver ingress errors never echo variable values.

package proxy

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func redactingRouter(m *Module, agent bool) *echo.Echo {
	router := echo.New()
	cfg := huma.DefaultConfig("contract", "test")
	cfg.Transformers = append(cfg.Transformers, ErrorTransformer)
	api := humaecho.NewWithGroup(router, router.Group("/api"), cfg)
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, &authz.PermissionSet{Sudo: true})))
	})
	m.RegisterRoutes(api, agent)
	return router
}

func asMap(t *testing.T, v any) map[string]any {
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func TestResolverValidationErrorsNeverEchoValues(t *testing.T) {
	const sentinel = "m28-sentinel-secret"
	x := newFixture(t)
	instance := x.request.Instance
	instance.EnvironmentID = "0"
	instance.Authority = dto.FileAuthority
	m := &Module{Agent: x.a, Settings: x.settings, Modules: x.mods, Hub: &Hub{DB: x.db.DB}}
	require.NoError(t, m.saveInstance(t.Context(), instance))
	base := dto.ResolverRequest{ResolverConfig: dto.ResolverConfig{Name: "test", Email: "admin@example.test", CA: dto.ResolverCA{Kind: "letsencrypt"}, Storage: "/certs/acme.json", Challenge: dto.ResolverChallenge{Kind: "dns", Provider: "httpreq"}}, Variables: []dto.ResolverVariable{}}

	variable := func(value string) map[string]any { return map[string]any{"name": "HTTPREQ_PASSWORD", "value": value} }
	bodies := map[string]func() map[string]any{
		"value too long": func() map[string]any {
			r := asMap(t, base)
			r["variables"] = []any{variable(sentinel + strings.Repeat("x", 4097))}
			return r
		},
		"too many variables": func() map[string]any {
			r := asMap(t, base)
			vars := []any{}
			for i := 0; i < 17; i++ {
				vars = append(vars, variable(sentinel))
			}
			r["variables"] = vars
			return r
		},
		"unexpected property": func() map[string]any {
			r := asMap(t, base)
			r["variables"] = []any{map[string]any{"name": "HTTPREQ_PASSWORD", "value": sentinel, "extra": sentinel}}
			return r
		},
		"variable without name": func() map[string]any {
			r := asMap(t, base)
			r["variables"] = []any{map[string]any{"value": sentinel}}
			return r
		},
		"missing name": func() map[string]any {
			r := asMap(t, base)
			delete(r, "name")
			r["variables"] = []any{variable(sentinel)}
			return r
		},
	}
	hub, agent := redactingRouter(m, false), redactingRouter(m, true)
	for name, body := range bodies {
		hubBody := body()
		w := postContract(hub, "/api/modules/proxy/instances/instance-one/resolvers", hubBody)
		require.Equal(t, 422, w.Code, "%s hub: %s", name, w.Body.String())
		require.NotContains(t, w.Body.String(), sentinel, "%s hub", name)
		require.Contains(t, w.Body.String(), string(dto.InvalidInput), "%s hub", name)

		apply := asMap(t, dto.ResolverApplyRequest{ID: "example", Instance: instance, Generation: instance.Generation, Registration: instance.Registration, Request: base})
		apply["request"] = body()
		w = postContract(agent, "/api/environments/0/modules/proxy/resolvers", apply)
		require.Equal(t, 422, w.Code, "%s agent: %s", name, w.Body.String())
		require.NotContains(t, w.Body.String(), sentinel, "%s agent", name)
		require.Contains(t, w.Body.String(), string(dto.InvalidInput), "%s agent", name)
	}
	for _, path := range []string{"/api/modules/proxy/instances/instance-one/resolvers", "/api/environments/0/modules/proxy/resolvers"} {
		router := hub
		if strings.HasPrefix(path, "/api/environments/") {
			router = agent
		}
		r := httptest.NewRequest("POST", path, strings.NewReader(`{"value":"`+sentinel+`"}`))
		r.Header.Set("Content-Type", "text/plain")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		require.Equal(t, 422, w.Code, "%s unsupported content type: %s", path, w.Body.String())
		require.NotContains(t, w.Body.String(), sentinel, path)
	}
}
