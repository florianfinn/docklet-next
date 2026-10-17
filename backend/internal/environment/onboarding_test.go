// Source: docklet-next/docs/design/host-onboarding.md
// Revision: 81439fab6a8ec5232161dc933fbd41bec41885fa
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original discovery and one-time credential HTTP tests.

package environment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

func TestHubDiscoveryRanksConfiguredURLAndPinsImage(t *testing.T) {
	cfg := &config.Config{AppUrl: "https://hub.example.com", Port: "3552"}
	discovery, err := hubDiscovery(cfg, "https://public.example.com/", "192.0.2.10:3552", []string{"192.0.2.20", "127.0.0.1", "192.0.2.20"})
	require.NoError(t, err)
	require.Equal(t, "https://public.example.com", discovery.Candidates[0].URL)
	require.Equal(t, "configured", discovery.Candidates[0].Source)
	require.Equal(t, "unknown", discovery.Candidates[0].Reach)
	require.Len(t, discovery.Candidates, 5)
	require.Equal(t, "http://127.0.0.1:3552", discovery.Candidates[3].URL)
	require.Equal(t, "same-host", discovery.Candidates[3].Reach)
	require.Equal(t, "interface", discovery.Candidates[4].Source)
	require.NotContains(t, discovery.AgentImage, "latest")
	require.True(t, pinnedAgentImage.MatchString(discovery.AgentImage))
	cfg.AgentImage = "registry.example.com/docklet/agent:1.2.3@sha256:" + strings.Repeat("a", 64)
	overridden, err := hubDiscovery(cfg, "", "", nil)
	require.NoError(t, err)
	require.Equal(t, cfg.AgentImage, overridden.AgentImage)
	for _, invalid := range []string{"agent:latest", "agent", "agent:1.2", "agent:1.2.3@sha256:abc"} {
		cfg.AgentImage = invalid
		_, err = hubDiscovery(cfg, "", "", nil)
		require.Error(t, err)
	}
}

func TestHubDiscoveryRejectsUnsafeAddressHints(t *testing.T) {
	cfg := &config.Config{AppUrl: "https://hub.example.com"}
	for _, invalid := range []string{"https://operator:example@hub.example.com", "file:///tmp/example", "https://hub.example.com/path", "https://hub.example.com?token=example"} {
		discovery, err := hubDiscovery(cfg, invalid, "", []string{"0.0.0.0", "invalid"})
		require.NoError(t, err)
		require.Len(t, discovery.Candidates, 1)
		require.Equal(t, cfg.AppUrl, discovery.Candidates[0].URL)
	}
}

func TestOnboardingHTTPTokenLifecycle(t *testing.T) {
	for _, edge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[edge], func(t *testing.T) {
			f := newAgentFixture(t, "direct", version.Info{})
			keys := apikey.NewApiKeyService(f.service.db, user.NewUserService(f.service.db, nil, nil), nil)
			f.service.apiKeys = keys
			h := &EnvironmentHandler{environmentService: f.service, apiKeyService: keys, cfg: &config.Config{}}
			mux := http.NewServeMux()
			api := humago.New(mux, huma.DefaultConfig("Onboarding", "0.0.0"))
			api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
				withUser := context.WithValue(ctx.Context(), usertypes.CurrentUserContextKey{}, &usertypes.Actor{ID: "operator", Username: "operator"})
				next(huma.WithContext(ctx, context.WithValue(withUser, middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
			})
			RegisterEnvironments(api, h)
			send := func(method, path, body string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				return rec
			}
			url := "http://agent.example.com:3552"
			if edge {
				url = "edge://pending"
			}
			body, err := json.Marshal(map[string]any{"name": "Example host", "apiUrl": url, "isEdge": edge, "useApiKey": true})
			require.NoError(t, err)
			created := send("POST", "/environments", string(body))
			require.Equal(t, 200, created.Code, created.Body.String())
			var payload struct {
				Data struct {
					ID     string `json:"id"`
					ApiKey string `json:"apiKey"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(created.Body.Bytes(), &payload))
			id, token := payload.Data.ID, payload.Data.ApiKey
			require.NotEmpty(t, token)
			for _, path := range []string{"/environments", "/environments/" + id} {
				listed := send("GET", path, "")
				require.Equal(t, 200, listed.Code, listed.Body.String())
				require.NotContains(t, listed.Body.String(), token)
				require.NotContains(t, listed.Body.String(), "apiKey")
				require.NotContains(t, listed.Body.String(), "accessToken")
			}
			require.Equal(t, 410, send("GET", "/environments/"+id+"/deployment", "").Code)
			rotated := send("PUT", "/environments/"+id, `{"regenerateApiKey":true}`)
			require.Equal(t, 200, rotated.Code, rotated.Body.String())
			require.NoError(t, json.Unmarshal(rotated.Body.Bytes(), &payload))
			require.NotEmpty(t, payload.Data.ApiKey)
			require.NotEqual(t, token, payload.Data.ApiKey)
			require.NotContains(t, send("GET", "/environments/"+id, "").Body.String(), payload.Data.ApiKey)
			_, err = keys.GetEnvironmentByApiKey(t.Context(), token)
			require.Error(t, err)
			removed := send("DELETE", "/environments/"+id, "")
			require.Equal(t, 200, removed.Code, removed.Body.String())
			require.Equal(t, 404, send("GET", "/environments/"+id, "").Code)
			_, err = f.service.ResolveEdgeSessionKey(t.Context(), id, payload.Data.ApiKey)
			require.Error(t, err)
			h.background.Wait()
		})
	}
}

func TestOnboardingHTTPRequiresGlobalAdmin(t *testing.T) {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Onboarding", "0.0.0"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		// Holding a delegated host-management permission does not make this caller an admin.
		permissions := &authz.PermissionSet{Global: map[string]struct{}{authz.PermEnvironmentsCreate: {}, authz.PermEnvironmentsUpdate: {}, authz.PermEnvironmentsDelete: {}}}
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, permissions)))
	})
	RegisterEnvironments(api, &EnvironmentHandler{})
	for _, route := range []struct{ method, path string }{
		{"POST", "/environments"}, {"PUT", "/environments/example"}, {"DELETE", "/environments/example"}, {"GET", "/hub/discovery"},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(route.method, route.path, strings.NewReader("{}")))
		require.Equal(t, 403, rec.Code)
	}
}
