// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; split durable execution helpers; M12 persisted volume lifecycle and legacy restore cut.

package backup

import (
	"context"
	"encoding/json/v2"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
)

// StartBackup persists a running backup and submits application-owned work.
func (s *Service) StartBackup(ctx context.Context, environmentID, volumeName string, user usertypes.Actor, request volume.CreateBackupRequest) (volume.BackupEntry, error) {
	entry, owner, plan, err := s.prepareCapture(ctx, volumeName, backuptypes.VolumeBackupTriggerManual, request)
	if err != nil {
		return volume.BackupEntry{}, err
	}
	activityID, workCtx := activitylib.StartHandlerActivity(owner.ctx, s.deps.Activity, environmentID, activitytypes.TypeResourceAction, "volume", volumeName, volumeName, &user, "Creating backup", "Creating volume backup", database.JSON{"action": "create_volume_backup", "backupId": entry.ID}, false)
	if activityID == "" {
		owner.Release(ctx)
		return volume.BackupEntry{}, s.completeBackupInternal(ctx, entry, errors.New("activity unavailable"))
	}
	entry.ActivityID = &activityID
	if err = s.deps.Store.Save(ctx, entry); err != nil {
		activitylib.CompleteHandlerActivity(workCtx, s.deps.Activity, activityID, "", err)
		owner.Release(ctx)
		return volume.BackupEntry{}, err
	}
	accepted := entryInternal(entry)
	go func() {
		defer owner.Release(workCtx)
		err := s.executeBackupInternal(workCtx, entry, user, plan)
		err = s.completeBackupInternal(workCtx, entry, err)
		activitylib.CompleteHandlerActivity(workCtx, s.deps.Activity, activityID, "Volume backup captured", neutralBackupError(err, "volume.backup.interrupted"))
	}()
	return accepted, nil
}

func (s *Service) ReconcileBackup(ctx context.Context, previous scheduler.Run, volumeName string) (scheduler.Outcome, error) {
	outcome := jobcontext.ConfirmedTarget(previous, volumeName)
	if outcome.Status == scheduler.Succeeded {
		return outcome, nil
	}
	return scheduler.Outcome{Status: scheduler.Failed, Message: "volume.backup.interrupted"}, nil
}

type manualVolumeBackupInternal struct {
	Checkpoint volumeBackupRecoveryInternal `json:"checkpoint"`
	VolumeName string                       `json:"volumeName"`
	UserID     string                       `json:"userId"`
	ActivityID string                       `json:"activityId"`
}

// executeDurableBackupInternal only settles pre-M12 persisted commands.
func (s *Service) executeDurableBackupInternal(ctx context.Context, _ string, payload []byte, _ bool) error {
	var command manualVolumeBackupInternal
	if err := json.Unmarshal(payload, &command); err != nil {
		return err
	}
	if s.isWriterFenced(command.VolumeName) {
		return backuptypes.NewProblem(409, "volume.backup.writer-unconfirmed", nil)
	}
	return s.deps.Store.Fail(context.WithoutCancel(ctx), command.Checkpoint.BackupID, "volume.backup.interrupted")
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
