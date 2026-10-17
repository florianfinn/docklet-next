// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package environment

import (
	"context"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/types/contract"
	environmenttypes "github.com/florianfinn/docklet-next/types/environment"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

func TestIncompatibleAgentBlocksAllSyncTriggers(t *testing.T) {
	for _, source := range []string{"creation", "update", "pairing", "health-job"} {
		t.Run(source, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			f := newAgentFixture(t, "direct", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: "different"})
			h := &EnvironmentHandler{environmentService: f.service}
			switch source {
			case "creation":
				_, err := h.createEnvironmentLegacyInternal(ctx, &Environment{ID: "created-host", ApiUrl: "http://agent.example.com", Enabled: true, Status: "pending"}, &usertypes.Actor{ID: "operator", Username: "operator"}, environmenttypes.Create{AccessToken: &f.token})
				require.NoError(t, err)
			case "update":
				_, err := h.UpdateEnvironment(ctx, &UpdateEnvironmentInput{ID: f.id, Body: environmenttypes.Update{Name: new("Changed")}})
				require.NoError(t, err)
			case "pairing":
				keys := apikey.NewApiKeyService(f.service.db, user.NewUserService(f.service.db, nil, nil), nil)
				key, err := keys.CreateEnvironmentApiKey(ctx, f.id)
				require.NoError(t, err)
				h.apiKeyService = keys
				require.NoError(t, f.service.db.Model(&Environment{}).Where("id = ?", f.id).Updates(map[string]any{"api_key_id": key.ID, "access_token": key.Key}).Error)
				_, err = h.PairEnvironment(ctx, &PairEnvironmentInput{XAPIKey: key.Key})
				require.NoError(t, err)
			case "health-job":
				gate := newAdmissionGateForEnvironmentTestInternal(t)
				require.NoError(t, f.service.SetScheduler(ctx, &environmentTestSchedulerInternal{}, gate))
				_, err := f.service.runHealthCheckInternal(ctx, f.id)
				require.Error(t, err)
			}
			done := make(chan struct{})
			go func() { h.background.Wait(); close(done) }()
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("sync trigger did not finish")
			}
			require.Positive(t, f.count("GET /api/app-version"))
			require.Zero(t, f.count("POST /api/container-registries/sync"))
			require.Zero(t, f.count("PUT /api/environments/0/settings"))
		})
	}
}
