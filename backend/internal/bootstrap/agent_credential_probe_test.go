// Source: docklet-next/issues/96
// Revision: 9695ef3e6b5a13155348997b86f513eb560efae8
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original protected system-health route and agent authentication regression.

package bootstrap

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/labstack/echo/v5"

	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

type credentialProbeRoundTrip func(*http.Request) (*http.Response, error)

func (f credentialProbeRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDirectCompatibilityProbeRequiresAgentCredential(t *testing.T) {
	probes := 0
	dockerClient, err := client.New(client.WithHost("tcp://docker.example.com:2375"), client.WithAPIVersion("1.53"),
		client.WithHTTPClient(&http.Client{Transport: credentialProbeRoundTrip(func(r *http.Request) (*http.Response, error) {
			probes++
			require.True(t, strings.HasSuffix(r.URL.Path, "/_ping"))
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Api-Version": {"1.53"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		})}))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, dockerClient.Close()) })
	cfg := &config.Config{AgentMode: true, AgentToken: "test-only", AppUrl: "https://agent.example.com", Environment: config.AppEnvironmentTest}
	router := echo.New()
	apiConfig := huma.DefaultConfig("test", "1.0.0")
	apiConfig.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(typ reflect.Type, hint string) string {
		return strings.NewReplacer("/", "_", ".", "_", "-", "_").Replace(typ.PkgPath()) + "_" + huma.DefaultSchemaNamer(typ, hint)
	})
	api := humaecho.NewWithGroup(router, router.Group("/api"), apiConfig)
	api.UseMiddleware(auth.NewHumaMiddleware(api, auth.NewAuthService(nil, nil, nil, nil, cfg), nil, nil, nil, cfg))
	system.RegisterSystem(api, system.NewHandler(&docker.DockerClientService{Client: dockerClient}, nil, t.Context()))
	for _, token := range []string{"", "example", cfg.AgentToken} {
		req := httptest.NewRequest(http.MethodHead, "/api/environments/0/system/health", nil)
		req.Header.Set(middleware.HeaderAgentToken, token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if token == cfg.AgentToken {
			require.Equal(t, http.StatusOK, rec.Code)
			require.Positive(t, probes)
		} else {
			require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, rec.Code)
			require.Zero(t, probes, "credential rejection happens before Docker access")
		}
	}
}
