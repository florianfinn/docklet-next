// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original persisted asynchronous restore lifecycle and inherited recovery ownership.

package restore

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/notification/backupnotify"
	"os"
	"path/filepath"
	"sync"
	"time"
	"uuid"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
)

// LifecycleEffects isolate Docker effects from durable phase transitions.
type LifecycleEffects struct {
	Validate    func(context.Context, StartRequest) error
	Admit       func(context.Context, string, string) (context.Context, func(), error)
	Prepare     func(context.Context, *Run, StartRequest) (func() error, error)
	Consumers   func(context.Context, string) ([]volume.StoppedContainer, error)
	Stop        func(context.Context, volume.StoppedContainer, user.Actor) error
	Restart     func(context.Context, []volume.StoppedContainer, user.Actor) error
	Safety      func(context.Context, string, user.Actor) (string, error)
	Write       func(context.Context, *Run, StartRequest) error
	CleanupNew  func(context.Context, *Run) error
	Fence       func(context.Context, string)
	PruneSafety func(context.Context, string) error
}
type Lifecycle struct {
	Notify   func(context.Context, backupnotify.Run)
	DB       *database.DB
	Activity *activity.ActivityService
	Effects  LifecycleEffects
}
type StartRequest struct {
	EnvironmentID, VolumeName, BackupID, Kind, InputPath string
	RestoreID                                            string
	Mode                                                 string
	NewVolumeName                                        string
	Selection                                            dto.RestoreSelection
	User                                                 user.Actor
}

func (l *Lifecycle) Start(ctx context.Context, request StartRequest) (volume.RestoreOperation, error) {
	if request.Mode == "" {
		request.Mode = "replace"
	}
	if request.Mode != "replace" && request.Mode != "new" || request.Kind == "files" && request.Mode == "new" {
		return volume.RestoreOperation{}, dto.NewProblem(422, "volume.backup.policy-invalid", nil)
	}
	if err := l.Effects.Validate(ctx, request); err != nil {
		return volume.RestoreOperation{}, err
	}
	target := ""
	if request.Mode == "new" {
		target = request.NewVolumeName
	}
	owned, release, err := l.Effects.Admit(ctx, request.VolumeName, target)
	if err != nil {
		return volume.RestoreOperation{}, err
	}
	release = sync.OnceFunc(release)
	id := request.RestoreID
	if id == "" {
		id = uuid.New().String()
	}
	run := &Run{ID: id, VolumeName: request.VolumeName, Kind: request.Kind, BackupID: request.BackupID, InputPath: request.InputPath, Mode: request.Mode, NewVolumeName: target, Phase: "preparing", Status: "running", StartedAt: time.Now().UTC()}
	if err = l.DB.WithContext(ctx).Create(run).Error; err != nil {
		release()
		return volume.RestoreOperation{}, err
	}
	activityID, workCtx := activitylib.StartHandlerActivity(owned, l.Activity, request.EnvironmentID, activitytypes.TypeResourceAction, "volume", request.VolumeName, request.VolumeName, &request.User, "Restoring volume", "Restoring volume data", database.JSON{"action": "restore_volume_backup", "restoreId": id}, false)
	if activityID == "" {
		run.Status = "failed"
		run.ErrorCode = "volume.restore.interrupted-before-write"
		run.FinishedAt = new(time.Now().UTC())
		l.persistTerminal(ctx, run)
		release()
		return volume.RestoreOperation{}, dto.NewProblem(503, "volume.backup.engine-unavailable", nil)
	}
	run.ActivityID = activityID
	if err = l.DB.WithContext(ctx).Save(run).Error; err != nil {
		run.Status = "failed"
		run.ErrorCode = "volume.restore.interrupted-before-write"
		run.FinishedAt = new(time.Now().UTC())
		l.persistTerminal(ctx, run)
		release()
		activitylib.CompleteHandlerActivity(workCtx, l.Activity, activityID, "", dto.NewProblem(500, "volume.restore.interrupted-before-write", err))
		return volume.RestoreOperation{}, err
	}
	go func() {
		defer release()
		err := l.Execute(workCtx, run, request)
		release()
		activitylib.CompleteHandlerActivity(workCtx, l.Activity, activityID, "Volume restored", err)
	}()
	return volume.RestoreOperation{ActivityID: activityID, RestoreID: id}, nil
}
func ownContainers(run *Run) []volume.StoppedContainer {
	var result []volume.StoppedContainer
	if run.StoppedContainers != nil {
		_ = json.Unmarshal([]byte(*run.StoppedContainers), &result)
	}
	return result
}
func (l *Lifecycle) persist(ctx context.Context, run *Run) error {
	return l.DB.WithContext(context.WithoutCancel(ctx)).Save(run).Error
}

// Terminal persistence finishes before admission or activity ownership can end.
func (l *Lifecycle) persistTerminal(ctx context.Context, run *Run) {
	for err := l.persist(ctx, run); err != nil; err = l.persist(ctx, run) {
		time.Sleep(100 * time.Millisecond)
	}
	if l.Notify != nil {
		l.Notify(ctx, backupnotify.Run{ID: run.ID, Operation: "restore", Volume: run.VolumeName, Status: run.Status, Reason: run.ErrorCode, ActivityID: run.ActivityID})
	}
}
func (l *Lifecycle) Execute(ctx context.Context, run *Run, request StartRequest) (err error) {
	cleanup := func() error { return nil }
	var inherited []Run
	writeSucceeded := false
	defer func() {
		if recovered := recover(); recovered != nil {
			err = dto.NewProblem(500, "volume.restore.interrupted", nil)
		}
		if cleanupErr := cleanup(); cleanupErr != nil {
			if l.Effects.Fence != nil {
				l.Effects.Fence(ctx, run.VolumeName)
			}
			err = dto.NewProblem(409, "volume.backup.writer-unconfirmed", cleanupErr)
			return
		}
		if run.InputPath != "" {
			_ = os.RemoveAll(filepath.Dir(run.InputPath))
		}
		settled := context.WithoutCancel(ctx)
		if writeSucceeded {
			restart := ownContainers(run)
			for i := range inherited {
				restart = append(restart, ownContainers(&inherited[i])...)
			}
			if restartErr := l.Effects.Restart(settled, restart, request.User); restartErr != nil {
				run.Status = "partial"
				run.ErrorCode = "volume.restore.restart-failed"
			} else {
				run.Status = "succeeded"
				run.ErrorCode = ""
			}
			if run.Mode != "new" {
				if e := l.DB.WithContext(settled).Model(&Run{}).Where("volume_name = ? AND pending_restart = ?", run.VolumeName, true).Update("pending_restart", false).Error; e != nil {
					err = e
				}
				if l.Effects.PruneSafety != nil {
					if e := l.Effects.PruneSafety(settled, run.VolumeName); e != nil {
						run.Status = "partial"
						run.ErrorCode = "volume.backup.retention-failed"
					}
				}
			}
			run.Phase = "done"
			run.PendingRestart = false
		} else if err != nil {
			run.Status = "failed"
			if run.Phase == "writing" {
				run.PendingRestart = true
				run.ErrorCode = "volume.restore.failed-partial"
				if ctx.Err() != nil {
					run.ErrorCode = "volume.restore.interrupted"
				}
			} else {
				if run.ErrorCode == "" {
					run.ErrorCode = "volume.restore.interrupted-before-write"
					var p *dto.Problem
					if errors.As(err, &p) {
						run.ErrorCode, run.ErrorReason = p.Code, p.Reason
					}
				}
				err = errors.Join(err, l.Effects.Restart(settled, ownContainers(run), request.User))
			}
			if run.Mode == "new" {
				run.PendingRestart = false
				if e := l.Effects.CleanupNew(settled, run); e != nil {
					run.CleanupPending = true
					err = errors.Join(err, e)
				}
			}
		}
		run.FinishedAt = new(time.Now().UTC())
		l.persistTerminal(settled, run)
		if err != nil {
			var problem *dto.Problem
			if errors.As(err, &problem) && problem.Code == run.ErrorCode {
				err = problem
			} else {
				err = dto.NewProblem(500, run.ErrorCode, err)
			}
		}
	}()
	if err = ctx.Err(); err != nil {
		return err
	}
	// Upload expansion and ENOSPC happen in scratch before consumer/data effects.
	if cleanup, err = l.Effects.Prepare(ctx, run, request); err != nil {
		if cleanup == nil {
			cleanup = func() error { return nil }
		}
		return err
	}
	if cleanup == nil {
		cleanup = func() error { return nil }
	}
	if run.Mode != "new" {
		if err = l.DB.WithContext(ctx).Where("volume_name = ? AND pending_restart = ?", run.VolumeName, true).Find(&inherited).Error; err != nil {
			return err
		}
		own, e := l.Effects.Consumers(ctx, run.VolumeName)
		if e != nil {
			return e
		}
		bytes, e := json.Marshal(own)
		if e != nil {
			return e
		}
		run.StoppedContainers = new(string(bytes))
		if err = l.persist(ctx, run); err != nil {
			return err
		}
		for _, container := range own {
			if err = l.Effects.Stop(ctx, container, request.User); err != nil {
				return err
			}
		}
		if !run.CreatedVolume {
			run.SafetyBackupID, err = l.Effects.Safety(ctx, run.VolumeName, request.User)
			if err != nil {
				run.ErrorCode = "volume.restore.safety-failed"
				return err
			}
			if err = l.persist(ctx, run); err != nil {
				return err
			}
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	run.Phase = "writing"
	if err = l.persist(ctx, run); err != nil {
		run.Phase = "preparing"
		return err
	}
	if err = l.Effects.Write(ctx, run, request); err != nil {
		return err
	}
	writeSucceeded = true
	return nil
}

// Reconcile never replays a write. restored history cannot authorize Docker effects.
func (l *Lifecycle) Reconcile(ctx context.Context, restored bool, fenced func(string) bool) error {
	var rows []Run
	if err := l.DB.WithContext(ctx).Where("status = ? OR pending_restart = ? OR cleanup_pending = ?", "running", true, true).Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		row := &rows[i]
		if fenced != nil && fenced(row.VolumeName) {
			continue
		}
		if restored {
			row.PendingRestart = false
			row.CleanupPending = false
		} else if row.Mode == "new" && (row.Status == "running" || row.CleanupPending) {
			row.CleanupPending = l.Effects.CleanupNew(ctx, row) != nil
		} else if row.Status == "running" && row.Phase == "preparing" {
			if err := l.Effects.Restart(ctx, ownContainers(row), user.SystemUser); err != nil {
				return err
			}
		}
		if row.Status == "running" {
			row.Status = "failed"
			row.ErrorCode = "volume.restore.interrupted-before-write"
			if row.Phase == "writing" {
				row.ErrorCode = "volume.restore.interrupted"
				row.PendingRestart = !restored && row.Mode != "new"
			}
			row.FinishedAt = new(time.Now().UTC())
		}
		if err := l.persist(ctx, row); err != nil {
			return err
		}
	}
	return nil
}
