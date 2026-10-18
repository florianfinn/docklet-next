// Source: getarcaneapp/arcane/backend/internal/environment/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; functional cut: removed Swarm; functional cut: removed S3 backup destinations.

package environment

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"go.getarcane.app/kit/pkg"
	"log/slog"
	"net/http"
	"time"
)

func (s *EnvironmentService) SubscribeRuntimeChanges() (<-chan struct{}, func()) {
	return s.runtimeWatchers.subscribe()
}

// NotifyRuntimeStateChanged wakes every runtime watcher.
func (s *EnvironmentService) NotifyRuntimeStateChanged() {
	if s == nil {
		return
	}
	s.runtimeWatchers.notify()
}

const (
	defaultEnvironmentHealthInterval        = "0 */2 * * * *"
	environmentHealthCheckTimeout           = 90 * time.Second
	environmentHealthJobPrefix              = "environment-health:"
	environmentHealthAdmissionScopeInternal = "environment-health"
)

// SetScheduler injects the job scheduler and app lifecycle context. Called during
// bootstrap on the manager only (agent mode leaves scheduler nil, so all health-job
// registration becomes a no-op).
func (s *EnvironmentService) SetScheduler(ctx context.Context, jobScheduler scheduler.DynamicScheduler, admissionGate *runs.Admission) error {
	return s.jobs.SetScheduler(ctx, jobScheduler, admissionGate)
}

// registerHealthJobInternal schedules one environment's health check on the
// global environmentHealthInterval (environment health has no per-entity
// interval); the run body self-cancels cleanly when the environment is gone.
func (s *EnvironmentService) registerHealthJobInternal(ctx context.Context, envID string) {
	s.jobs.Register(ctx, envID,
		func(ctx context.Context) string {
			sched := s.settingsService.GetStringSetting(ctx, "environmentHealthInterval", defaultEnvironmentHealthInterval)
			return kit.Ternary(sched == "", defaultEnvironmentHealthInterval, sched)
		},
		func(ctx context.Context) (scheduler.Outcome, error) { return s.runHealthCheckInternal(ctx, envID) },
		func(context.Context, scheduler.Run) (scheduler.Outcome, error) {
			return scheduler.Outcome{Status: scheduler.Retrying, Message: "Connection checks can safely resume"}, nil
		},
	)
}

func (s *EnvironmentService) removeHealthJobInternal(ctx context.Context, envID string) {
	s.jobs.Unregister(ctx, envID)
}

func (s *EnvironmentService) ListEnabledEnvironmentIDs(ctx context.Context) ([]string, error) {
	var ids []string
	if err := s.db.WithContext(ctx).
		Table("environments").
		Where("enabled = ?", true).
		Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("failed to list enabled environments: %w", err)
	}
	return ids, nil
}

func (s *EnvironmentService) registerAllEnabledHealthJobsInternal(ctx context.Context) int {
	ids, err := s.ListEnabledEnvironmentIDs(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list environments for health jobs", "error", err)
		return 0
	}
	for _, id := range ids {
		s.registerHealthJobInternal(ctx, id)
	}
	return len(ids)
}

// RegisterHealthJobsOnStartup registers a health-check job for every enabled
// environment. Replaces the old global environment-health job.
func (s *EnvironmentService) RegisterHealthJobsOnStartup(ctx context.Context) {
	if !s.jobs.Enabled() {
		return
	}
	n := s.registerAllEnabledHealthJobsInternal(ctx)
	slog.InfoContext(ctx, "Registered environment health jobs on startup", "count", n)
}

// RescheduleHealthJobs re-registers all enabled environments' health jobs, picking
// up a changed global interval. Wired from the Jobs UI via job.JobService.
func (s *EnvironmentService) RescheduleHealthJobs(ctx context.Context) {
	if !s.jobs.Enabled() {
		return
	}
	s.registerAllEnabledHealthJobsInternal(ctx)
}

// RunHealthChecksNow queues each enabled environment's health check.
// Child runs retain the parent identity so authorization is checked at execution.
func (s *EnvironmentService) RunHealthChecksNow(ctx context.Context) error {
	if !s.jobs.Enabled() {
		return errors.New("environment health scheduler unavailable")
	}
	ids, err := s.ListEnabledEnvironmentIDs(ctx)
	if err != nil {
		return err
	}
	parent, hasParent := jobcontext.Run(ctx)
	request := scheduler.Request{EnvironmentID: "0", Trigger: "internal"}
	if hasParent {
		request.Trigger = parent.Trigger
		request.RequestedBy = parent.RequestedBy
		request.RequestedWithKey = parent.RequestedWithKey
	}
	for _, id := range ids {
		request.JobID = s.jobs.JobName(id)
		if _, submitErr := s.jobs.Scheduler().Submit(ctx, request); submitErr != nil {
			return submitErr
		}
	}
	return nil
}

// runHealthCheckInternal tests one environment's connection (updating its DB status)
// and syncs registries only to compatible remote agents.
func (s *EnvironmentService) runHealthCheckInternal(ctx context.Context, envID string) (scheduler.Outcome, error) {
	lease, admitted, err := s.jobs.TryAcquire(ctx, envID)
	if err != nil {
		slog.ErrorContext(ctx, "environment health check admission failed", "environmentId", envID, "error", err)
		return scheduler.Outcome{}, err
	}
	if !admitted {
		slog.WarnContext(ctx, "environment health check skipped; previous run still in progress", "environmentId", envID)
		return scheduler.Outcome{Status: scheduler.Skipped}, nil
	}
	defer lease.Release(ctx)

	localEnvironment, err := s.GetEnvironmentByID(ctx, envID)
	if err != nil {
		return scheduler.Outcome{}, err
	}
	if !localEnvironment.Enabled {
		return scheduler.Outcome{Status: scheduler.Canceled, Message: "Environment disabled"}, nil
	}
	wasOnline := localEnvironment.Status == string(EnvironmentStatusOnline)
	status, err := s.TestConnection(ctx, envID, nil)
	switch {
	case status == string(EnvironmentStatusUpdateRequired):
		return scheduler.Outcome{Status: scheduler.Partial, Message: "Agent update required"}, ErrAgentUpdateRequired
	case err != nil:
		slog.WarnContext(ctx, "environment health check failed", "environmentId", envID, "status", status, "error", err)
		return scheduler.Outcome{Status: scheduler.Retrying}, err
	case status != "online":
		return scheduler.Outcome{Status: scheduler.Retrying, Message: "Environment is offline"}, errors.New("environment is offline")
	}

	// Local environment (ID "0") has no configuration to push.
	if envID == "0" {
		return scheduler.Outcome{Status: scheduler.Succeeded}, nil
	}

	// An agent that just came back may have restarted with different state;
	// resend everything once instead of trusting the last delivered payload.
	if !wasOnline {
		s.ForgetSyncState(envID)
	}

	var syncErrors []error
	syncCtx, cancel := context.WithTimeout(ctx, environmentHealthCheckTimeout)
	defer cancel()
	if syncRegistriesToEnvironmentErr := s.SyncRegistriesToEnvironment(syncCtx, envID); syncRegistriesToEnvironmentErr != nil {
		slog.WarnContext(syncCtx, "failed to sync registries during health check", "environmentId", envID, "error", syncRegistriesToEnvironmentErr)
		syncErrors = append(syncErrors, syncRegistriesToEnvironmentErr)
	}
	if joinErr := errors.Join(syncErrors...); joinErr != nil {
		return scheduler.Outcome{Status: scheduler.Partial}, joinErr
	}
	return scheduler.Outcome{Status: scheduler.Succeeded}, nil
}

// SyncRegistriesToEnvironment syncs all registries from this manager to a remote environment.
func (s *EnvironmentService) SyncRegistriesToEnvironment(ctx context.Context, environmentID string) error {
	target, err := s.resolveRemoteEnvironmentTargetInternal(ctx, environmentID)
	if err != nil {
		return err
	}
	return s.proxy.SyncRegistries(ctx, target)
}

func (s *EnvironmentService) TestConnection(ctx context.Context, id string, customApiUrl *string) (string, error) {
	envRecord, err := s.GetEnvironmentByID(ctx, id)
	if err != nil {
		return "error", err
	}

	if id != "0" && customApiUrl == nil {
		reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		response, probeErr := s.ExecuteRemoteRequest(reqCtx, id, http.MethodGet, "/api/health", nil)
		if probeErr != nil {
			_ = s.updateEnvironmentStatusInternal(ctx, id, "offline")
			return "offline", probeErr
		}
		if err := response.RequireSuccess(); err != nil {
			_ = s.updateEnvironmentStatusInternal(ctx, id, "offline")
			return "offline", err
		}
		if _, err := s.CheckAgentCompatibility(reqCtx, id); err != nil {
			if errors.Is(err, ErrAgentUpdateRequired) {
				return string(EnvironmentStatusUpdateRequired), nil
			}
			if !envRecord.IsEdge && errors.Is(err, remenv.ErrAgentInvalidToken) {
				return string(EnvironmentStatusTokenRejected), err
			}
			_ = s.updateEnvironmentStatusInternal(ctx, id, "offline")
			return "offline", err
		}
		return "online", nil
	}
	apiURL := envRecord.ApiUrl
	if customApiUrl != nil {
		apiURL = *customApiUrl
	}
	status, err := s.health.Probe(ctx, id, apiURL, envRecord.IsEdge, customApiUrl != nil)
	if customApiUrl == nil {
		_ = s.updateEnvironmentStatusInternal(ctx, id, status)
		return status, err
	}
	if err != nil {
		slog.WarnContext(ctx, "Environment custom URL connection test failed", "environmentId", id, "error", err)
		return "error", common.ErrEnvironmentConnectionTestFailed
	}
	return status, nil
}

func (s *EnvironmentService) updateEnvironmentStatusInternal(ctx context.Context, id, status string) error {
	var currentEnv Environment
	if err := s.db.WithContext(ctx).Select("status", "is_edge").Where("id = ?", id).First(&currentEnv).Error; err != nil {
		return fmt.Errorf("failed to check environment status: %w", err)
	}

	if currentEnv.Status == string(EnvironmentStatusPending) {
		// Edge envs must complete pairing via the agent's outbound tunnel — manager
		// can't dial them directly, so a manager-side reachability check means nothing.
		// Direct envs are reachable from the manager, so a successful health check IS
		// the pairing signal. Don't promote on offline/error ticks though, or a transient
		// blip during initial setup would flip the env out of pending.
		if currentEnv.IsEdge || status != string(EnvironmentStatusOnline) {
			slog.DebugContext(ctx, "skipping status update for pending environment", "environmentId", id)
			return nil
		}
		slog.InfoContext(ctx, "promoted pending direct environment to online via reachability check", "environmentId", id)
	}

	now := time.Now()
	updates := map[string]any{
		"status": status,

		"updated_at": &now,
	}
	if status == string(EnvironmentStatusOnline) || status == string(EnvironmentStatusUpdateRequired) {
		updates["last_seen"] = &now
	}
	if err := s.db.WithContext(ctx).Model(&Environment{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update environment status: %w", err)
	}
	// Direct environments have no tunnel callback, so this health-check write is
	// the only moment their liveness changes.
	s.NotifyRuntimeStateChanged()
	return nil
}

func (s *EnvironmentService) UpdateEnvironmentHeartbeat(ctx context.Context, id string) error {
	if id != "0" {
		_, err := s.CheckAgentCompatibility(ctx, id)
		return err
	}
	now := time.Now()

	// Use Exec with raw SQL for better performance
	// Only update if last_seen is NULL or older than 30 seconds to reduce write frequency
	result := s.db.WithContext(ctx).Exec(`
		UPDATE environments
		SET last_seen = ?, status = ?, updated_at = ?
		WHERE id = ?
		AND (last_seen IS NULL OR last_seen < ?)
	`, new(now), string(EnvironmentStatusOnline), new(now), id, now.Add(-30*time.Second))

	if result.Error != nil {
		// The 30s throttle above doubles as the notify throttle: a no-op heartbeat
		// changed nothing worth waking a stream for.
		return fmt.Errorf("failed to update environment heartbeat: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		s.NotifyRuntimeStateChanged()
	}

	return nil
}

// UpdateEnvironmentConnectionState updates runtime connectivity status without creating
// a generic "environment updated" event. This is used for edge tunnel connect/disconnect.
func (s *EnvironmentService) UpdateEnvironmentConnectionState(ctx context.Context, id string, connected bool) error {
	now := time.Now()

	updates := map[string]any{
		"updated_at": &now,
	}
	if connected {
		updates["status"] = string(EnvironmentStatusPending)
		updates["last_seen"] = &now
		// Remember the tunnel transport so the UI can keep showing it after
		// the tunnel drops or while the agent is poll-only.
		if state, ok := edge.GetTunnelRuntimeState(id).Get(); ok && state.Transport != "" {
			updates["last_edge_transport"] = state.Transport
		}
	} else {
		var current Environment
		if err := s.db.WithContext(ctx).Select("status").Where("id = ?", id).First(&current).Error; err != nil {
			return err
		}
		if current.Status != string(EnvironmentStatusPending) {
			updates["status"] = string(EnvironmentStatusOffline)
		}
	}

	if err := s.db.WithContext(ctx).Model(&Environment{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update environment connection state: %w", err)
	}

	s.invalidateEnvironmentCacheInternal(id)
	s.NotifyRuntimeStateChanged()

	return nil
}
