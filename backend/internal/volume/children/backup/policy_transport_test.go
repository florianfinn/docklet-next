// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original hub-to-agent policy validation through authenticated target sync and HTTP routes.

package backup

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

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
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
)

func TestPolicyInvalidPayloadsForwardedByHubNeverChangeAgentPolicy(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) { policyTransportValidation(t, mode) })
	}
}
func policyTransportValidation(t *testing.T, mode string) {
	hub, agent := newRuntimePlan(t), newRuntimePlan(t)
	for _, f := range []*runtimePlan{hub, agent} {
		require.NoError(t, f.service.deps.DB.AutoMigrate(&backuptarget.Target{}, &backuptarget.Check{}, &policies.VolumeBackupPolicy{}))
		f.service.deps.Store.Latest = func(context.Context, string) (*volume.Backup, error) { return nil, nil }
	}
	token := strings.Repeat("s", 32)
	zone, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	agent.service.policies = policies.NewService(policies.Dependencies{DB: agent.service.deps.DB, Location: zone, ValidateTarget: agent.service.validatePolicyTarget, LatestRun: agent.service.latestRunInternal})
	nextRun := time.Date(2026, 11, 1, 7, 0, 0, 0, zone)
	sched := &backupSchedulerProof{jobs: map[string]scheduler.Job{}, next: nextRun}
	require.NoError(t, agent.service.policies.SetScheduler(t.Context(), sched, newVolumeAdmissionForTestInternal(t)))
	router, rawAPI := humatest.New(t)
	api := huma.NewGroup(rawAPI, "/api")
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	cfg := &config.Config{AgentMode: mode == "direct", EdgeAgent: mode == "edge", AgentToken: token}
	backuptarget.RegisterHostRoutes(api, TargetHandler(agent.service, nil, cfg, t.Context()), true)
	agentPolicies := policies.NewHandler(agent.service.policies, nil, nil, t.Context())
	agentPolicies.AroundUpdate = agent.service.aroundPolicyUpdate(nil, cfg)
	agentPolicies.BeforeLocalUpdate = agent.service.checkPolicySet
	policies.RegisterRoutes(api, agentPolicies)
	requests := 0
	agentHTTP := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	})
	server := httptest.NewServer(agentHTTP)
	defer server.Close()
	id := uuid.New().String()
	endpoint := server.URL
	if mode == "edge" {
		backupRouteTunnel(t, agentHTTP, edge.GetRegistry(), id, token)
		endpoint = "edge://example"
	}
	require.NoError(t, hub.service.deps.DB.AutoMigrate(&environment.Environment{}))
	require.NoError(t, hub.service.deps.DB.Create(&environment.Environment{BaseModel: database.BaseModel{ID: id}, Name: "Agent", ApiUrl: endpoint, Enabled: true, IsEdge: mode == "edge", AccessToken: &token}).Error)
	environments := environment.NewEnvironmentService(hub.service.deps.DB, server.Client(), nil, nil, nil, nil)
	h := policies.NewHandler(hub.service.policies, nil, environments, t.Context())
	h.AroundUpdate = hub.service.aroundPolicyUpdate(environments, &config.Config{})
	valid := volume.UpdateBackupPolicy{Enabled: true, LocalEnabled: true, Schedule: "0 0 2 * * *", RetentionCount: 7, TargetID: "local"}
	first, err := h.UpdateBackupPolicy(t.Context(), &policies.UpdateVolumeBackupPolicyInput{EnvironmentID: id, VolumeName: "data", Body: volume.UpdateBackupPolicies{Policies: []volume.UpdateBackupPolicy{valid}}})
	if err != nil {
		t.Logf("sync cause: %v", errors.Unwrap(err))
	}
	require.NoError(t, err)
	require.Len(t, first.Body.Data.Policies, 1)
	require.Equal(t, "Asia/Tokyo", first.Body.Data.TimeZone)
	read, err := h.GetBackupPolicy(t.Context(), &policies.GetVolumeBackupPolicyInput{EnvironmentID: id, VolumeName: "data"})
	require.NoError(t, err)
	require.Equal(t, "Asia/Tokyo", read.Body.Data.TimeZone)
	require.NotNil(t, read.Body.Data.Policies[0].NextRunAt)
	require.True(t, nextRun.Equal(*read.Body.Data.Policies[0].NextRunAt))
	valid.ID = first.Body.Data.Policies[0].ID
	stale, err := json.Marshal(volume.UpdateBackupPolicies{Authority: "old-authority", SetVersion: 1, Policies: []volume.UpdateBackupPolicy{valid}})
	require.NoError(t, err)
	call := backupRouteTransport(t, mode, agentHTTP)
	status, _, body := call(t.Context(), http.MethodPut, "/api/environments/0/volumes/data/backup-policy", stale)
	require.Equal(t, 409, status, string(body))
	require.Contains(t, string(body), "backup.target.stale-set")
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
			_, err := h.UpdateBackupPolicy(t.Context(), &policies.UpdateVolumeBackupPolicyInput{EnvironmentID: id, VolumeName: "data", Body: volume.UpdateBackupPolicies{Policies: updates}})
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
