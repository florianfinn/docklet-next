// Source: getarcaneapp/arcane/backend/internal/system/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Extracted Hub policy scheduling; logs successful registration.

package backup

import (
	"context"
	"errors"
	"log/slog"

	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	usertypes "github.com/florianfinn/docklet-next/types/user"
)

const defaultSystemBackupSchedule = "0 0 3 * * *"

func (s *Service) SetScheduler(ctx context.Context, dynamicScheduler scheduler.DynamicScheduler, admissionGate *runs.Admission) error {
	return s.jobs.SetScheduler(ctx, dynamicScheduler, admissionGate)
}
func (s *Service) reschedulePolicy(ctx context.Context, policy *backuptypes.SystemBackupPolicy) {
	if policy == nil {
		return
	}
	if !policy.Enabled {
		s.jobs.Unregister(ctx, policy.ID)
		return
	}
	id := policy.ID
	if s.jobs.Register(ctx, id, func(ctx context.Context) string {
		current, err := s.store.Policy(ctx, id)
		if err != nil || current == nil {
			return defaultSystemBackupSchedule
		}
		return current.Schedule
	}, func(ctx context.Context) (scheduler.Outcome, error) { return s.runScheduledBackup(ctx, id) },
		func(ctx context.Context, previous scheduler.Run) (scheduler.Outcome, error) {
			// A crashed backup never resumes. Startup storage reconciliation removes its partial file.
			return scheduler.Outcome{Status: scheduler.Failed}, nil
		}) {
		slog.InfoContext(ctx, "Registered scheduled Hub backup job", "policyId", id, "schedule", policy.Schedule)
	}
}
func (s *Service) RegisterBackupJobOnStartup(ctx context.Context) {
	policies, err := s.store.Policies(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to load Hub backup policies", "code", "backup.failed")
		return
	}
	for i := range policies {
		s.reschedulePolicy(ctx, &policies[i])
	}
	s.volumes.RegisterJobsOnStartup(ctx)
}
func (s *Service) runScheduledBackup(ctx context.Context, id string) (scheduler.Outcome, error) {
	policy, err := s.store.Policy(ctx, id)
	if err != nil {
		return scheduler.Outcome{}, err
	}
	if policy == nil || !policy.Enabled {
		return scheduler.Outcome{Status: scheduler.Skipped}, nil
	}
	run, err := s.CreateBackup(ctx, usertypes.SystemUser, backuptypes.SystemBackupTriggerScheduled,
		backuptypes.CreateSystemBackupRequest{PolicyID: id})
	if errors.Is(err, ErrSystemBackupAlreadyRunning) {
		return scheduler.Outcome{Status: scheduler.Skipped}, nil
	}
	if errors.Is(err, ErrSystemBackupRetention) {
		return scheduler.Outcome{Status: scheduler.Partial, ActivityID: run.ActivityID, Message: "backup.retention_failed"}, nil
	}
	if err != nil {
		return scheduler.Outcome{}, err
	}
	_ = jobcontext.Progress(ctx, scheduler.TargetOutcome{ResourceType: "system_backup", ID: id, Status: scheduler.Succeeded, ActivityID: run.ActivityID})
	return scheduler.Outcome{Status: scheduler.Succeeded, ActivityID: run.ActivityID}, nil
}
