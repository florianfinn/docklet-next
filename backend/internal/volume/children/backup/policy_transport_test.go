// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original hub-to-agent policy validation through authenticated target sync and HTTP routes.

package backup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
)

func TestPolicyInvalidPayloadsForwardedByHubNeverChangeAgentPolicy(t *testing.T) {
	hub, agent := newRuntimePlan(t), newRuntimePlan(t)
	for _, f := range []*runtimePlan{hub, agent} {
		require.NoError(t, f.service.deps.DB.AutoMigrate(&backuptarget.Target{}, &backuptarget.Check{}, &policies.VolumeBackupPolicy{}))
		f.service.deps.Store.Latest = func(context.Context, string) (*volume.Backup, error) { return nil, nil }
	}
	token := strings.Repeat("s", 32)
	router, rawAPI := humatest.New(t)
	api := huma.NewGroup(rawAPI, "/api")
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	cfg := &config.Config{AgentMode: true, AgentToken: token}
	backuptarget.RegisterHostRoutes(api, TargetHandler(agent.service, nil, cfg, t.Context()), true)
	agentPolicies := policies.NewHandler(agent.service.policies, nil, nil, t.Context())
	agentPolicies.AroundUpdate = agent.service.aroundPolicyUpdate(nil, cfg)
	agentPolicies.BeforeLocalUpdate = agent.service.checkPolicySet
	policies.RegisterRoutes(api, agentPolicies)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/app-version" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
			return
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(204)
			return
		}
		requests++
		router.ServeHTTP(w, r)
	}))
	defer server.Close()
	require.NoError(t, hub.service.deps.DB.AutoMigrate(&environment.Environment{}))
	require.NoError(t, hub.service.deps.DB.Create(&environment.Environment{BaseModel: database.BaseModel{ID: "agent"}, Name: "Agent", ApiUrl: server.URL, Enabled: true, AccessToken: &token}).Error)
	environments := environment.NewEnvironmentService(hub.service.deps.DB, server.Client(), nil, nil, nil, nil)
	h := policies.NewHandler(hub.service.policies, nil, environments, t.Context())
	h.AroundUpdate = hub.service.aroundPolicyUpdate(environments, &config.Config{})
	valid := volume.UpdateBackupPolicy{Enabled: true, LocalEnabled: true, Schedule: "0 0 2 * * *", RetentionCount: 7, TargetID: "local"}
	first, err := h.UpdateBackupPolicy(t.Context(), &policies.UpdateVolumeBackupPolicyInput{EnvironmentID: "agent", VolumeName: "data", Body: volume.UpdateBackupPolicies{Policies: []volume.UpdateBackupPolicy{valid}}})
	require.NoError(t, err)
	require.Len(t, first.Body.Data.Policies, 1)
	valid.ID = first.Body.Data.Policies[0].ID
	var original policies.VolumeBackupPolicy
	require.NoError(t, agent.service.deps.DB.First(&original, "volume_name = ?", "data").Error)
	for _, tc := range []struct {
		name     string
		change   func(*volume.UpdateBackupPolicy)
		multiple bool
	}{
		{"multiple", func(*volume.UpdateBackupPolicy) {}, true},
		{"five-field", func(p *volume.UpdateBackupPolicy) { p.Schedule = "0 2 * * *" }, false},
		{"cron-range", func(p *volume.UpdateBackupPolicy) { p.Schedule = "0 90 2 * * *" }, false},
		{"negative-retention", func(p *volume.UpdateBackupPolicy) { p.RetentionCount = -1 }, false},
		{"large-retention", func(p *volume.UpdateBackupPolicy) { p.RetentionCount = 3651 }, false},
		{"disabled-destination", func(p *volume.UpdateBackupPolicy) { p.LocalEnabled = false }, false},
		{"foreign-id", func(p *volume.UpdateBackupPolicy) { p.ID = "other-policy" }, false},
		{"unknown-target", func(p *volume.UpdateBackupPolicy) { p.TargetID = "unknown" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			update := valid
			tc.change(&update)
			updates := []volume.UpdateBackupPolicy{update}
			if tc.multiple {
				updates = append(updates, update)
			}
			_, err := h.UpdateBackupPolicy(t.Context(), &policies.UpdateVolumeBackupPolicyInput{EnvironmentID: "agent", VolumeName: "data", Body: volume.UpdateBackupPolicies{Policies: updates}})
			require.Error(t, err)
			var current policies.VolumeBackupPolicy
			require.NoError(t, agent.service.deps.DB.First(&current, "volume_name = ?", "data").Error)
			require.Equal(t, original, current, "refused input cannot partially change the owning host's policy")
		})
	}
	require.Greater(t, requests, 8, "the owning agent must receive the forwarded payloads")
	require.Empty(t, hub.creates)
	require.Empty(t, agent.creates)
}
