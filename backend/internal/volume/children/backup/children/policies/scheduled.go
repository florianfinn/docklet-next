// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/children/policies/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; single owning-host policy, target selection and scheduler-produced next-run metadata; M12 backup foundation; stable codes instead of diagnostics on policy failures, including the scheduled activity and target progress; M18 live scheduling, policy and contract foundation.

package policies

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"gorm.io/gorm"
	"log/slog"
)

func (s *Service) runScheduledBackupInternal(ctx context.Context, policyID string) (scheduler.Outcome, error) {
	var policy VolumeBackupPolicy
	if err := s.deps.DB.WithContext(ctx).Where("id = ? AND enabled = ?", policyID, true).First(&policy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return scheduler.Outcome{Status: scheduler.Canceled, Message: "Backup policy disabled or deleted"}, nil
		}
		return scheduler.Outcome{}, publicPolicyErrorInternal(err, 500)
	}
	if previous, ok := jobcontext.Run(ctx); ok {
		outcome := jobcontext.ConfirmedTarget(previous, policy.VolumeName)
		if outcome.Status == scheduler.Succeeded {
			return outcome, nil
		}
	}
	if s.deps.Acquire != nil {
		workCtx, release, err := s.deps.Acquire(ctx, policy.VolumeName)
		if errors.Is(err, s.deps.AlreadyRunning) {
			return scheduler.Outcome{Status: scheduler.Waiting, Code: "run.busy"}, nil
		}
		if err != nil {
			return scheduler.Outcome{}, publicPolicyErrorInternal(err, 500)
		}
		defer release()
		ctx = workCtx
	}
	// The activity and both target progress writes see only the public error.
	activityID, workCtx := activitylib.StartHandlerActivity(ctx, s.deps.Activity, "0", activitytypes.TypeResourceAction, "volume_backup", policy.VolumeName, policy.VolumeName, &user.SystemUser,
		"Creating scheduled backup", "Creating scheduled volume backup", database.JSON{
			"action":         "scheduled_volume_backup",
			"policyId":       policy.ID,
			"schedule":       policy.Schedule,
			"volumeName":     policy.VolumeName,
			"retentionCount": policy.RetentionCount,
			"stopContainers": policy.StopContainers,
			"localEnabled":   policy.LocalEnabled,
		}, false)
	var entry *volume.Backup
	err := jobcontext.Progress(workCtx, scheduler.TargetOutcome{ID: policy.VolumeName, Status: scheduler.Running, ActivityID: activityID})
	if err == nil {
		entry, err = s.deps.CreateBackup(workCtx, policy.VolumeName, policy.ID)
	}
	public := publicPolicyErrorInternal(err, 500)
	activitylib.CompleteHandlerActivity(workCtx, s.deps.Activity, activityID, "Scheduled volume backup created successfully", public)
	target := scheduler.TargetOutcome{ID: policy.VolumeName, Status: scheduler.Succeeded, ActivityID: activityID}
	if public != nil {
		target.Status = scheduler.Failed
		target.Message = public.Error()
	}
	if progressErr := jobcontext.Progress(workCtx, target); progressErr != nil && public == nil {
		err, public = progressErr, publicPolicyErrorInternal(progressErr, 500)
	}
	if errors.Is(err, s.deps.AlreadyRunning) {
		slog.InfoContext(ctx, "Scheduled volume backup waiting; another backup is running", "volume", policy.VolumeName)
		return scheduler.Outcome{Status: scheduler.Waiting, Code: "run.busy"}, nil
	}
	if public != nil {
		slog.ErrorContext(ctx, "Scheduled volume backup failed", "volume", policy.VolumeName, "error", err)
		return scheduler.Outcome{}, public
	}
	slog.InfoContext(ctx, "Scheduled volume backup completed", "volume", policy.VolumeName, "backupId", entry.ID)
	return scheduler.Outcome{Status: scheduler.Succeeded, ActivityID: activityID}, nil
}
