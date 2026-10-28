// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original asynchronous repository tests with admission-time configuration and safe results.

package backup

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"uuid"

	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"gorm.io/gorm/clause"
)

func (s *Service) hostID(ctx context.Context) (string, error) {
	var value struct{ Value string }
	if err := s.deps.DB.WithContext(ctx).Table("settings").Where("key = ?", "hubHostId").Scan(&value).Error; err != nil {
		return "", err
	}
	if value.Value == "" {
		return "0", nil
	}
	return value.Value, nil
}
func (s *Service) startRepositoryCheck(ctx context.Context, id string) (dto.TargetTestOperation, error) {
	actor, err := handlerutil.RequireUser(ctx)
	if err != nil {
		return dto.TargetTestOperation{}, err
	}
	host, err := s.hostID(ctx)
	if err != nil {
		return dto.TargetTestOperation{}, err
	}
	version := int64(0)
	configVersion := int64(0)
	if id != "local" {
		target, err := s.targetStore.Resolve(ctx, id, host, true)
		if err != nil {
			return dto.TargetTestOperation{}, err
		}
		configVersion = target.ConfigVersion
		ctx = context.WithValue(ctx, admittedTargetKey{}, target)
	}
	if host == "0" {
		_, version, err = s.targetStore.Authority(ctx)
	} else {
		var value struct{ Value int64 }
		err = s.deps.DB.WithContext(ctx).Table("settings").Where("key = ?", "backupTargetSetVersion").Scan(&value).Error
		version = value.Value
	}
	if err != nil {
		return dto.TargetTestOperation{}, err
	}
	scope := "repository:" + id
	if err = s.checkWriterFence(ctx, scope); err != nil {
		return dto.TargetTestOperation{}, err
	}
	lease, admitted, err := s.deps.Engine.TryAcquireRun(ctx, "volume-backup", scope)
	if err != nil {
		return dto.TargetTestOperation{}, dto.NewProblem(503, "volume.backup.engine-unavailable", err)
	}
	if !admitted {
		return dto.TargetTestOperation{}, dto.NewProblem(409, "volume.backup.busy", s.deps.AlreadyRunning)
	}
	ctx = rustic.WithOperation(ctx, scope, uuid.New().String())
	activityID, workCtx := activitylib.StartHandlerActivity(ctx, s.deps.Activity, "0", activitytypes.TypeResourceAction, "backup_target", id, id, actor, "Checking backup repository", "Checking backup repository", database.JSON{"action": "check_volume_backup_repository", "targetId": id}, false)
	if activityID == "" {
		lease.Release(ctx)
		return dto.TargetTestOperation{}, dto.NewProblem(503, "volume.backup.engine-unavailable", nil)
	}
	check := backuptarget.Check{TargetID: id, HostID: host, SetVersion: version, ConfigVersion: configVersion, ActivityID: activityID, At: time.Now().UTC()}
	// Persist the admitted config before the worker mounts anything. A later edit
	// must never make an older worker's successful test look current.
	if id != "local" {
		if err = s.deps.DB.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&check).Error; err != nil {
			lease.Release(ctx)
			activitylib.CompleteHandlerActivity(workCtx, s.deps.Activity, activityID, "", neutralBackupError(err, "volume.backup.repository-check-failed"))
			return dto.TargetTestOperation{}, err
		}
	}
	go func() {
		defer lease.Release(context.WithoutCancel(workCtx))
		worker, cancel := context.WithTimeout(workCtx, 5*time.Minute)
		defer cancel()
		count, err := s.checkRepository(worker, id)
		result := volume.BackupRepositoryCheck{OK: err == nil, At: time.Now().UTC(), Initialized: err == nil}
		if err != nil {
			var p *dto.Problem
			if errors.As(err, &p) {
				result.Code = p.Code
			} else {
				result.Code = "volume.backup.repository-check-failed"
			}
		} else {
			result.SnapshotCount = &count
		}
		check.OK, check.Code, check.At, check.SnapshotCount = result.OK, result.Code, result.At, result.SnapshotCount
		if id == "local" {
			payload, _ := json.Marshal(result)
			err = errors.Join(err, s.deps.DB.WithContext(context.WithoutCancel(worker)).Exec("INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", "volumeBackupRepositoryCheck", string(payload)).Error)
		} else {
			err = errors.Join(err, s.deps.DB.WithContext(context.WithoutCancel(worker)).Model(&backuptarget.Check{}).Where("target_id = ? AND host_id = ? AND activity_id = ?", id, host, activityID).Updates(map[string]any{"ok": check.OK, "code": check.Code, "at": check.At, "snapshot_count": check.SnapshotCount}).Error)
		}
		projection := dto.TargetCheck{HostID: host, SetVersion: check.SetVersion, ConfigVersion: check.ConfigVersion, ActivityID: activityID, OK: check.OK, Code: check.Code, SnapshotCount: check.SnapshotCount, At: check.At}
		err = errors.Join(err, s.deps.Activity.PatchActivityMetadata(workCtx, activityID, database.JSON{"backupTargetId": id, "backupTargetCheck": projection}))
		lease.Release(context.WithoutCancel(workCtx))
		activitylib.CompleteHandlerActivity(workCtx, s.deps.Activity, activityID, "Backup repository checked", neutralBackupError(err, cmp.Or(result.Code, "volume.backup.repository-check-failed")))
	}()
	return dto.TargetTestOperation{ActivityID: activityID}, nil
}
func (s *Service) checkRepository(ctx context.Context, id string) (int64, error) {
	cli, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return 0, dto.NewProblem(503, "volume.backup.engine-unavailable", err)
	}
	repository, err := s.repositoryFor(ctx, cli, id, false)
	if err != nil {
		return 0, err
	}
	if repository.BeforeRun != nil {
		if err = repository.BeforeRun(ctx); err != nil {
			return 0, err
		}
	}
	image, err := s.deps.HelperImage(ctx, cli)
	if err != nil {
		return 0, err
	}
	root := ""
	for _, value := range repository.Environment {
		if strings.HasPrefix(value, "RUSTIC_REPOSITORY=") {
			root = strings.TrimPrefix(value, "RUSTIC_REPOSITORY=")
		}
	}
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{Image: image, Cmd: []string{"sleep", "infinity"}, Labels: rustic.OperationLabels(ctx)}, HostConfig: volumehelper.HostConfig(image, nil, repository.Mounts)})
	if err != nil {
		return 0, dto.NewProblem(503, "backup.target.mount-failed", err)
	}
	// Keep the repository lease while removal is unconfirmed. This also keeps a
	// maintenance drain from replacing the DB under a surviving helper.
	defer func() {
		for {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			_, _ = cli.ContainerRemove(cleanup, created.ID, volumehelper.RemoveOptions())
			_, err := cli.ContainerInspect(cleanup, created.ID, client.ContainerInspectOptions{})
			cancel()
			if errdefs.IsNotFound(err) {
				return
			}
			time.Sleep(time.Second)
		}
	}()
	if _, err = cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return 0, dto.NewProblem(503, "backup.target.mount-failed", err)
	}
	probeOutput, _, err := s.deps.Exec(ctx, created.ID, "", []string{"sh", "-c", `set -e; root="$1"; probe="$root/.docklet-probe-$2"; mkdir -p "$root"; state=empty; for entry in "$root"/* "$root"/.[!.]* "$root"/..?*; do if test -e "$entry" || test -L "$entry"; then state=present; break; fi; done; printf docklet > "$probe"; value=$(head -c 7 "$probe"); test "$value" = docklet; rm -f -- "$probe"; printf '%s' "$state"`, "sh", root, uuid.New().String()})
	if err != nil {
		return 0, dto.NewProblem(422, "backup.target.permission-denied", err)
	}
	password, err := s.volumeBackupPasswordInternal(ctx, cli, repository)
	if err != nil {
		return 0, err
	}
	state := strings.TrimSpace(probeOutput)
	if state != "empty" && state != "present" {
		return 0, dto.NewProblem(422, "volume.backup.engine-output-invalid", nil)
	}
	return s.deps.Engine.CheckRepository(ctx, cli, repository, password, state == "empty")
}
