// Source: getarcaneapp/arcane/backend/internal/system/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Extracted hub backup admission; best-effort activity archive naming for M41 S3.

package backup

import (
	"context"
	"errors"
	"log/slog"
	"time"
	"uuid"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	sharedbackup "github.com/florianfinn/docklet-next/backend/internal/backup"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"
)

func (s *Service) startRun(ctx context.Context, actor usertypes.Actor, trigger string, request backuptypes.CreateSystemBackupRequest) (*backuptypes.SystemBackupRun, context.Context, error) {
	if err := s.validateRequest(ctx, request); err != nil {
		return nil, nil, err
	}
	copyTargetID := ""
	if request.PolicyID != "" {
		policy, err := s.store.Policy(ctx, request.PolicyID)
		if err != nil {
			return nil, nil, err
		}
		if policy == nil {
			return nil, nil, ErrSystemBackupNotFound
		}
		copyTargetID = policy.TargetID
	}
	id := "system-" + uuid.New().String()
	answer, err := s.activityService.StartGatedActivity(ctx, activity.GateRequest{
		EnvironmentID: "0", Operation: "create:" + id, Scope: sharedbackup.SystemAdmissionScope,
		Type: activitytypes.TypeResourceAction, ResourceType: "system_backup", ResourceID: id,
		StartedBy: &actor,
	})
	if err != nil {
		var conflict *activity.GateConflict
		if errors.As(err, &conflict) {
			return nil, nil, ErrSystemBackupAlreadyRunning
		}
		return nil, nil, err
	}
	name, _ := backuptypes.ArchiveNameForID(id) // The generated UUID is a valid archive ID.
	s.setActivityArchiveName(answer.WorkContext, answer.Run.ID, name)
	run := &backuptypes.SystemBackupRun{ID: id, ActivityID: answer.Run.ID,
		CreatedAt: time.Now().UTC(), Status: backuptypes.SystemBackupStatusRunning,
		Trigger: trigger, PolicyID: request.PolicyID, CopyTargetID: copyTargetID, CopyState: backuptypes.SystemBackupCopyNone}
	if err := s.store.CreateRun(ctx, run); err != nil {
		code := "backup.failed"
		_, _ = s.activityService.CompleteActivity(answer.WorkContext, answer.Run.ID, activitytypes.StatusFailed, code, &code)
		return nil, nil, err
	}
	return run, answer.WorkContext, nil
}

// Naming is best-effort and does not change the admitted backup operation.
func (s *Service) setActivityArchiveName(ctx context.Context, id, name string) {
	if _, err := s.activityService.UpdateActivity(ctx, id, activity.UpdateActivityRequest{ResourceName: &name}); err != nil {
		slog.WarnContext(ctx, "failed to update backup activity resource name", "error", err)
	}
}
