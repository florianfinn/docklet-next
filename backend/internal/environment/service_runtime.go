// Source: getarcaneapp/arcane/backend/internal/environment/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; functional cut: removed Swarm; functional cut: removed S3 backup destinations; application-heartbeat liveness and observer clock projection; polling agents below the minimum protocol keep update-required.

package environment

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/environment"
	"log/slog"
	"time"
)

// ApplyEnvironmentRuntimeState normalizes edge environment runtime status using
// in-memory tunnel and poll registries without mutating persisted state.
func ApplyEnvironmentRuntimeState(env *environment.Environment, expectedKey ...string) {
	applyEnvironmentRuntimeStateAt(env, time.Now(), expectedKey...)
}

func applyEnvironmentRuntimeStateAt(env *environment.Environment, now time.Time, expectedKey ...string) {
	if env == nil || !env.IsEdge {
		return
	}

	connected := false
	env.Connected = &connected
	env.ConnectedAt = nil
	env.LastHeartbeat = nil
	env.LastPollAt = nil
	env.EdgeTransport = nil
	env.EdgeSecurityMode = nil
	env.EdgeSessionID = nil
	env.EdgeAgentInstance = nil
	env.EdgeCapabilities = nil

	if pollState, ok := edge.GetPollRuntimeRegistry().Get(env.ID, now).Get(); ok {
		env.LastPollAt = pollState.LastPollAt
	}

	keyCurrent := true
	if len(expectedKey) > 0 {
		tunnel, ok := edge.GetRegistry().Get(env.ID).Get()
		keyCurrent = ok && tunnel != nil && tunnel.APIKeyID == expectedKey[0]
		if keyCurrent && len(expectedKey) > 1 {
			keyCurrent = tunnel.TokenHash == expectedKey[1]
		}
	}
	if runtimeState, ok := edge.GetTunnelRuntimeStateAt(env.ID, now).Get(); ok && keyCurrent {
		connected = true
		env.Connected = &connected
		if env.Status != string(EnvironmentStatusUpdateRequired) && env.Status != string(EnvironmentStatusPending) {
			env.Status = string(EnvironmentStatusOnline)
		}
		env.ConnectedAt = runtimeState.ConnectedAt
		env.LastHeartbeat = runtimeState.LastHeartbeat
		if runtimeState.SecurityMode != "" {
			env.EdgeSecurityMode = &runtimeState.SecurityMode
		}
		if runtimeState.SessionID != "" {
			env.EdgeSessionID = &runtimeState.SessionID
		}
		if runtimeState.AgentInstance != "" {
			env.EdgeAgentInstance = &runtimeState.AgentInstance
		}
		if len(runtimeState.Capabilities) > 0 {
			env.EdgeCapabilities = append([]string(nil), runtimeState.Capabilities...)
		}
		if transport, localOk := edge.GetActiveTunnelTransport(env.ID).Get(); localOk {
			env.EdgeTransport = &transport
		} else if runtimeState.Transport != "" {
			env.EdgeTransport = &runtimeState.Transport
		}
		return
	}

	if env.LastPollAt != nil {
		// A polling agent below the minimum protocol stays update-required, as on a tunnel.
		if env.Status != string(EnvironmentStatusUpdateRequired) {
			env.Status = string(EnvironmentStatusStandby)
		}
		return
	}

	if env.Status != string(EnvironmentStatusPending) {
		env.Status = string(EnvironmentStatusOffline)
	}
}

// ReconcileEdgeStatusesOnStartup resets edge environments to offline when the manager starts.
// Live edge tunnels are process-local runtime state, so persisted "online" flags can be stale
// after a restart until agents reconnect. Pending environments are left untouched.
func (s *EnvironmentService) ReconcileEdgeStatusesOnStartup(ctx context.Context) error {
	result := s.db.WithContext(ctx).Model(&Environment{}).
		Where("is_edge = ?", true).
		Where("status <> ?", string(EnvironmentStatusPending)).
		Where("status <> ?", string(EnvironmentStatusOffline)).
		Updates(map[string]any{
			"status":     string(EnvironmentStatusOffline),
			"updated_at": new(time.Now()),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to reconcile edge environment statuses: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		slog.InfoContext(ctx, "Reconciled stale edge environment statuses on startup", "count", result.RowsAffected)
	}

	return nil
}

// SyncRegistriesToRemoteEnvironments syncs container registries to all eligible remote environments.
// Eligibility requires a non-local, enabled environment with a configured access token.
func (s *EnvironmentService) SyncRegistriesToRemoteEnvironments(ctx context.Context) error {
	envs, err := s.ListRemoteEnvironments(ctx)
	if err != nil {
		return fmt.Errorf("failed to list remote environments for registry sync: %w", err)
	}

	if len(envs) == 0 {
		return nil
	}

	var failedCount int
	for _, env := range envs {
		if env.AccessToken == nil || *env.AccessToken == "" {
			slog.DebugContext(ctx, "Skipping registry sync for environment without access token",
				"environmentId", env.ID,
				"environmentName", env.Name)
			continue
		}

		if syncRegistriesToEnvironmentErr := s.SyncRegistriesToEnvironment(ctx, env.ID); syncRegistriesToEnvironmentErr != nil {
			failedCount++
			slog.WarnContext(ctx, "Failed to sync registries to remote environment",
				"environmentId", env.ID,
				"environmentName", env.Name,
				"error", syncRegistriesToEnvironmentErr.Error())
		}
	}

	if failedCount > 0 {
		return fmt.Errorf("failed to sync registries to %d remote environment(s)", failedCount)
	}

	return nil
}

// ApplyHostRuntimeState projects process-local reports without persisting stale data.
func (s *EnvironmentService) ApplyHostRuntimeState(ctx context.Context, env *environment.Environment) {
	s.ApplyHostRuntimeStateAt(ctx, env, time.Now())
}

// ApplyHostRuntimeStateAt shares the observer clock with the heartbeat predicate.
func (s *EnvironmentService) ApplyHostRuntimeStateAt(ctx context.Context, env *environment.Environment, now time.Time) {
	row, err := s.GetEnvironmentByID(ctx, env.ID)
	if err != nil {
		env.Status = string(EnvironmentStatusOffline)
		return
	}
	keyID := ""
	if row.ApiKeyID != nil {
		keyID = *row.ApiKeyID
	}
	tokenHash := ""
	if row.AccessToken != nil {
		tokenHash = edge.AgentTokenHash(*row.AccessToken)
	}
	applyEnvironmentRuntimeStateAt(env, now, keyID, tokenHash)
	s.agentRuntime.mu.Lock()
	report, ok := s.agentRuntime.reports[env.ID]
	s.agentRuntime.mu.Unlock()
	// Historical report data is display-only; session admission checks current credentials.
	if ok {
		copy := report.report
		env.AgentReport = &copy
	}
}

// SyncBackupTargetsToEnvironment sends metadata at the same lifecycle points
// as registry delivery. Mount and repository effects belong to admitted work.
func (s *EnvironmentService) SyncBackupTargetsToEnvironment(ctx context.Context, id string) error {
	if id == "0" {
		return nil
	}
	_, err := backuptarget.SyncHost(ctx, s.db, id, s.ProxyJSONRequest)
	return err
}
