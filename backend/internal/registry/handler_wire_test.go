// Source: docklet-next/docs/design/images-updates.md
// Revision: 7df52f8f3b912be751e2283ec8d990933f5dc4b8
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original image/registry UI foundation follow-up for issue #229.

package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

func registryWireRouter(s *ContainerRegistryService, permissions *authz.PermissionSet) *echo.Echo {
	router := echo.New()
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.SetRequest(c.Request().WithContext(context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, permissions)))
			return next(c)
		}
	})
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("registry", "test"))
	RegisterContainerRegistries(api, NewHandler(s, nil))
	return router
}

func registryWireRequest(router *echo.Echo, method, suffix, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/container-registries"+suffix, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRegistryHandlerWireUsesNeutralTypedFailures(t *testing.T) {
	for _, test := range []struct {
		method, path, body, code string
		status                   int
	}{
		{"GET", "/missing", "", "registry.not-found", 404},
		{"PUT", "/missing", `{}`, "registry.not-found", 404},
		{"POST", "", `{"url":"https://credential-identity:synthetic-private@registry.example.com"}`, "registry.invalid-config", 400},
	} {
		t.Run(test.method+test.path, func(t *testing.T) {
			s := NewContainerRegistryService(setupContainerRegistryTestDBInternal(t), nil, nil, nil)
			rec := registryWireRequest(registryWireRouter(s, authz.SudoPermissionSet()), test.method, test.path, test.body)
			require.Equal(t, test.status, rec.Code, rec.Body.String())
			var problem imageDTO.OperationError
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
			require.Equal(t, test.code, problem.Problem.Code)
			require.NotContains(t, rec.Body.String(), "synthetic-private")
		})
	}
}

func TestRegistryProbeWireSeparatesUpstreamAuthFromHubPermissions(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			db := setupContainerRegistryTestDBInternal(t)
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = fmt.Fprint(w, `{"message":"synthetic-private daemon diagnostic"}`)
			}))
			defer daemon.Close()
			c, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.41"))
			require.NoError(t, err)
			defer c.Close()
			s := NewContainerRegistryService(db, func(context.Context) (RegistryDaemonClient, error) { return c, nil }, nil, nil)
			reg, err := s.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{URL: "https://registry.example.com", Username: "credential-identity", Token: uuid.New().String()})
			require.NoError(t, err)
			router := registryWireRouter(s, authz.SudoPermissionSet())
			rec := registryWireRequest(router, "POST", "/"+reg.ID+"/test", "")
			want, code := 502, "registry.authentication-failed"
			if status == 429 {
				want, code = 429, "registry.rate-limited"
			}
			if status == 500 {
				want, code = 503, "registry.unavailable"
			}
			require.Equal(t, want, rec.Code, rec.Body.String())
			var result containerregistry.RegistryTestResult
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
			require.Equal(t, code, result.Code)
			require.False(t, result.OK)
			require.False(t, result.CheckedAt.IsZero())
			require.NotContains(t, rec.Body.String(), "synthetic-private")
			rec = registryWireRequest(registryWireRouter(s, authz.NewPermissionSet()), "POST", "/"+reg.ID+"/test", "")
			require.Equal(t, 403, rec.Code, "actual hub denial retains its status")
		})
	}
}

func TestRegistryProbeClearedCredentialWireIsNotHubAuthentication(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	require.NoError(t, db.Create(&ContainerRegistry{ID: "cleared", URL: "https://registry.example.com", RegistryType: "generic", Enabled: true}).Error)
	s := NewContainerRegistryService(db, nil, nil, nil)
	rec := registryWireRequest(registryWireRouter(s, authz.SudoPermissionSet()), "POST", "/cleared/test", "")
	require.Equal(t, 502, rec.Code, rec.Body.String())
	var result containerregistry.RegistryTestResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	require.Equal(t, "registry.authentication-required", result.Code)
	require.False(t, result.OK)
}

func TestRegistryHandlersKeepDatabaseFailuresTyped(t *testing.T) {
	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			db := setupContainerRegistryTestDBInternal(t)
			require.NoError(t, db.Migrator().DropTable(&ContainerRegistry{}))
			s := NewContainerRegistryService(db, nil, nil, nil)
			path, body := "/missing", `{}`
			if method == "GET" {
				path = ""
			}
			if method == "POST" {
				request, err := json.Marshal(containerregistry.CreateContainerRegistryRequest{URL: "https://registry.example.com", Username: "credential-identity", Token: uuid.New().String()})
				require.NoError(t, err)
				path, body = "", string(request)
			}
			rec := registryWireRequest(registryWireRouter(s, authz.SudoPermissionSet()), method, path, body)
			require.Equal(t, 503, rec.Code, rec.Body.String())
			var problem imageDTO.OperationError
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
			require.Equal(t, "registry.unavailable", problem.Problem.Code)
			require.Equal(t, "The operation could not be completed.", problem.Problem.Message)
			require.NotContains(t, rec.Body.String(), "container_registries")
		})
	}
}
