// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; split durable execution helpers.

package backup

import (
	"context"
	"encoding/json/v2"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	userdomain "github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/client"
	"gorm.io/gorm"
)

// StartBackup persists a running backup and submits application-owned work.
func (s *Service) StartBackup(ctx context.Context, environmentID, volumeName string, user usertypes.Actor, request volume.CreateBackupRequest) (volume.BackupEntry, error) {
	plan, err := s.resolveBackupPlanInternal(ctx, volumeName, backuptypes.VolumeBackupTriggerManual, request, nil)
	if err != nil {
		return volume.BackupEntry{}, err
	}
	entry, lease, err := s.prepareBackupInternal(ctx, volumeName, backuptypes.VolumeBackupTriggerManual, request.PolicyID, plan)
	if err != nil {
		return volume.BackupEntry{}, err
	}
	activityID, workCtx := activitylib.StartHandlerActivity(ctx, s.deps.Activity, environmentID, activitytypes.TypeResourceAction,
		"volume", volumeName, volumeName, &user, "Creating backup", "Creating volume backup",
		database.JSON{"action": "create_volume_backup", "backupId": entry.ID, "destination": entry.Destination, "policyId": request.PolicyID}, false)
	if activityID == "" {
		defer lease.Release(ctx)
		return volume.BackupEntry{}, s.completeBackupInternal(ctx, entry, errors.New("failed to start backup activity"))
	}
	entry.ActivityID = &activityID
	accepted := entryInternal(entry)
	keyID, _ := ctx.Value(middleware.ContextKeyApiKeyID).(string)
	payload, err := json.Marshal(
		manualVolumeBackupInternal{
			Checkpoint: volumeBackupRecoveryInternal{
				BackupID:     entry.ID,
				LocalEnabled: plan.localEnabled,
				Policy:       plan.policy,
			},
			VolumeName: volumeName,
			UserID:     user.ID,
			ActivityID: activityID,
		},
	)
	if err == nil {
		err = s.deps.Engine.SubmitDurableRun(
			workCtx,
			backuptypes.DurableRunCommand{
				Kind:             "volume",
				RunID:            entry.ID,
				ActivityID:       activityID,
				Payload:          payload,
				UserID:           user.ID,
				EnvironmentID:    environmentID,
				Permission:       authz.PermVolumesBackup,
				RequestedWithKey: keyID,
			},
			lease,
		)
	}
	if err != nil {
		lease.Release(ctx)
		err = s.completeBackupInternal(workCtx, entry, err)
		activitylib.CompleteHandlerActivity(workCtx, s.deps.Activity, activityID, "Volume backup created successfully", err)
		return volume.BackupEntry{}, err
	}

	return accepted, nil
}

func (s *Service) ReconcileBackup(ctx context.Context, previous scheduler.Run, volumeName string) (scheduler.Outcome, error) {
	outcome := jobcontext.ConfirmedTarget(previous, volumeName)
	if outcome.Status == scheduler.Succeeded {
		return outcome, nil
	}
	for _, target := range previous.Outcome.Targets {
		if target.ID != volumeName || len(target.RecoveryData) == 0 {
			continue
		}
		var checkpoint volumeBackupRecoveryInternal
		if err := json.Unmarshal(target.RecoveryData, &checkpoint); err != nil {
			return outcome, err
		}
		entry, loadErr := s.deps.Store.Run(ctx, checkpoint.BackupID)
		if loadErr != nil {
			return outcome, loadErr
		}
		if entry.VolumeName != volumeName {
			return outcome, gorm.ErrRecordNotFound
		}
		if entry.Status != backuptypes.VolumeBackupStatusSucceeded {
			if checkpoint.Policy != nil && checkpoint.Policy.StopContainers {
				return outcome, nil
			}
			lease, admitted, err := s.deps.Engine.AcquireDurableRun(ctx, previous.ID, backup.VolumeAdmissionScope, volumeName)
			if err != nil {
				return outcome, err
			}
			if !admitted {
				return outcome, nil
			}
			defer lease.Release(ctx) //nolint:gocritic // The matching recovery target always returns before the loop advances.
			if resumeBackupErr := s.resumeBackupInternal(ctx, previous, entry, checkpoint); resumeBackupErr != nil {
				return outcome, resumeBackupErr
			}
		}
		target.Status = scheduler.Succeeded
		if err := jobcontext.Progress(ctx, target); err != nil {
			return outcome, err
		}
		return scheduler.Outcome{Status: scheduler.Succeeded, Targets: []scheduler.TargetOutcome{target}}, nil
	}
	return outcome, nil
}

func (
	s *Service,
) recoverSnapshotInternal(
	ctx context.Context,
	dockerClient *client.Client,
	repository backup.Repository,
	entry *volume.Backup,
	snapshotID string,
) (
	backup.Snapshot,
	bool,
	error,
) {
	password, err := s.volumeBackupPasswordInternal(ctx, dockerClient, repository)
	if err != nil {
		return backup.Snapshot{}, false, err
	}
	return s.deps.Engine.FindRunSnapshot(ctx, dockerClient, repository, password, entry.ID, snapshotID)
}

func (s *Service) observeBackupDestinationInternal(ctx context.Context, dockerClient *client.Client, previous scheduler.Run, entry *volume.Backup) (backup.Repository, error) {
	local, err := s.localRusticRepositoryInternal(ctx, dockerClient, false)
	if err != nil {
		return local, err
	}
	if entry.LocalSnapshotID != "" || backupDestinationAttemptedInternal(previous, entry.ID, "local") {
		snapshot, found, err := s.recoverSnapshotInternal(ctx, dockerClient, local, entry, entry.LocalSnapshotID)
		if err != nil {
			return local, err
		}
		if found {
			entry.LocalSnapshotID, entry.Size = snapshot.ID, snapshot.Size
		}
	}
	return local, nil
}

func (s *Service) resumeBackupInternal(ctx context.Context, previous scheduler.Run, entry *volume.Backup, checkpoint volumeBackupRecoveryInternal) error {
	dockerClient, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return err
	}
	_, err = s.observeBackupDestinationInternal(ctx, dockerClient, previous, entry)
	if err != nil {
		return err
	}
	if operationErr := s.deps.Store.Save(ctx, entry); operationErr != nil {
		return operationErr
	}
	plan := backupPlanInternal{
		localEnabled: checkpoint.LocalEnabled,
		policy:       checkpoint.Policy,
		destination:  entry.Destination,
	}
	err = s.executeBackupInternal(ctx, entry, usertypes.SystemUser, plan)
	return s.completeBackupInternal(ctx, entry, err)
}

type manualVolumeBackupInternal struct {
	Checkpoint volumeBackupRecoveryInternal `json:"checkpoint"`
	VolumeName string                       `json:"volumeName"`
	UserID     string                       `json:"userId"`
	ActivityID string                       `json:"activityId"`
}

func (s *Service) executeDurableBackupInternal(ctx context.Context, runID string, payload []byte, interrupted bool) (err error) {
	var command manualVolumeBackupInternal
	if decodeErr := json.Unmarshal(payload, &command); decodeErr != nil {
		return decodeErr
	}
	defer func() {
		if ctx.Err() == nil {
			if err != nil {
				saveErr := s.deps.Store.Fail(ctx, command.Checkpoint.BackupID, err.Error())
				err = errors.Join(err, saveErr)
			}
			activitylib.CompleteHandlerActivity(ctx, s.deps.Activity, command.ActivityID, "Volume backup created successfully", err)
		}
	}()
	entry, loadBackupErr := s.deps.Store.Run(ctx, command.Checkpoint.BackupID)
	if loadBackupErr != nil {
		return loadBackupErr
	}

	if entry.Status == backuptypes.VolumeBackupStatusSucceeded {
		return nil
	}
	if interrupted {
		previous, _ := jobcontext.Run(ctx)
		outcome, recoveryErr := s.ReconcileBackup(ctx, previous, command.VolumeName)
		if recoveryErr != nil {
			return recoveryErr
		}
		if outcome.Status != scheduler.Succeeded {
			return errors.New(outcome.Message)
		}
		return nil
	}
	lease, admitted, err := s.deps.Engine.AcquireDurableRun(ctx, runID, backup.VolumeAdmissionScope, command.VolumeName)
	if err != nil {
		return err
	}
	if !admitted {
		return s.deps.AlreadyRunning
	}
	defer lease.Release(ctx)
	actor := usertypes.SystemUser
	if command.UserID != "" && command.UserID != "agent" {
		var localUser userdomain.User
		if loadUserErr := s.deps.DB.WithContext(ctx).Where("id = ?", command.UserID).First(&localUser).Error; loadUserErr != nil {
			return loadUserErr
		}
		actor = *localUser.Actor()
	}
	checkpoint, err := json.Marshal(command.Checkpoint)
	if err != nil {
		return err
	}
	if progressErr := jobcontext.Progress(
		ctx,
		scheduler.TargetOutcome{
			ResourceType: "volume_backup",
			ID:           command.VolumeName,
			Status:       scheduler.Running,
			RecoveryData: checkpoint,
			ActivityID:   command.ActivityID,
		},
	); progressErr != nil {
		return progressErr
	}
	plan := backupPlanInternal{
		localEnabled: command.Checkpoint.LocalEnabled,
		policy:       command.Checkpoint.Policy,
		destination:  entry.Destination,
	}
	err = s.executeBackupInternal(ctx, entry, actor, plan)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return s.completeBackupInternal(ctx, entry, err)
}

func (s *Service) failDurableBackupInternal(ctx context.Context, _ string, payload []byte, runErr error) error {
	var command manualVolumeBackupInternal
	if err := json.Unmarshal(payload, &command); err != nil {
		return err
	}
	entry, err := s.deps.Store.Run(ctx, command.Checkpoint.BackupID)
	if err != nil {
		return err
	}
	err = s.deps.Store.Fail(ctx, entry.ID, runErr.Error())
	activitylib.CompleteHandlerActivity(ctx, s.deps.Activity, command.ActivityID, "Volume backup created successfully", runErr)
	return err
}
