// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package environment

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	environmenttypes "github.com/florianfinn/docklet-next/types/environment"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestEdgeCreationRequiresAPIKey(t *testing.T) {
	for _, mode := range []string{"edge-flag", "edge-url", "missing-token"} {
		t.Run(mode, func(t *testing.T) {
			f := newAgentFixture(t, "direct", version.Info{})
			h := &EnvironmentHandler{environmentService: f.service}
			env := &Environment{ApiUrl: "edge://example", IsEdge: mode != "edge-url", Enabled: true}
			if mode == "edge-flag" {
				env.ApiUrl = "http://agent.example.com"
			}
			token := &f.token
			if mode == "missing-token" {
				token = nil
			}
			_, err := h.createEnvironmentLegacyInternal(t.Context(), env, &usertypes.Actor{ID: "operator", Username: "operator"}, environmenttypes.Create{AccessToken: token})
			var apiErr huma.StatusError
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.GetStatus())
			require.Contains(t, err.Error(), "useApiKey=true")
			h.background.Wait()
			var count int64
			require.NoError(t, f.service.db.Model(&Environment{}).Count(&count).Error)
			require.EqualValues(t, 1, count, "rejected creation must not persist a host")
		})
	}
}

func TestEdgeCreationWithAPIKeyRemainsAvailable(t *testing.T) {
	f := newAgentFixture(t, "direct", version.Info{})
	keys := apikey.NewApiKeyService(f.service.db, user.NewUserService(f.service.db, nil, nil), nil)
	h := &EnvironmentHandler{environmentService: f.service, apiKeyService: keys}
	out, err := h.createEnvironmentWithApiKeyInternal(t.Context(), &Environment{ApiUrl: "edge://example", IsEdge: true, Enabled: true}, &usertypes.Actor{ID: "operator", Username: "operator"})
	require.NoError(t, err)
	require.NotNil(t, out.Body.Data.ApiKey)
	env, err := f.service.GetEnvironmentByID(t.Context(), out.Body.Data.ID)
	require.NoError(t, err)
	require.NotNil(t, env.ApiKeyID)
	key, err := f.service.ResolveEdgeSessionKey(t.Context(), env.ID, *out.Body.Data.ApiKey)
	require.NoError(t, err)
	require.Equal(t, *env.ApiKeyID, key)
}
