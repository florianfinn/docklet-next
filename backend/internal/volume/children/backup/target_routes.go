// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original owning-host target handlers and transaction-local pairing fence.

package backup

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
	"gorm.io/gorm"
)

func TargetHandler(s *Service, environments *environment.EnvironmentService, cfg *config.Config, appCtx context.Context) *backuptarget.Handler {
	h := &backuptarget.Handler{}
	if s == nil {
		return h
	}
	h.Service = s.targets
	if (cfg == nil || !cfg.AgentMode && !cfg.EdgeAgent) && environments != nil {
		s.configureTargetDelivery(environments)
	}
	h.Apply = func(ctx context.Context, set dto.TargetSet, token string) (dto.TargetSetReceipt, error) {
		s.policies.AdmissionLock()
		defer s.policies.AdmissionUnlock()
		store := *s.targetStore
		store.SessionCurrent = func(ctx context.Context, tx *gorm.DB) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			fallback := ""
			if cfg != nil {
				fallback = cfg.AgentToken
			}
			return environment.AgentPairingTokenCurrent(tx, token, fallback)
		}
		return store.Apply(ctx, set)
	}
	h.References = s.targetReferences
	h.LocalKey = func(ctx context.Context) (string, error) { return s.volumeBackupPasswordInternal(ctx, nil) }
	h.KeyViewed = func(ctx context.Context, id string) error {
		actor, err := handlerutil.RequireUser(ctx)
		if err != nil {
			return err
		}
		if s.deps.Events == nil {
			return fmt.Errorf("backup audit unavailable")
		}
		return s.deps.Events.LogVolumeEvent(ctx, event.EventTypeVolumeBackupTargetKeyViewed, id, id, actor.ID, actor.Username, "0", database.JSON{"targetId": id})
	}
	h.Test = func(ctx context.Context, id, hostID string) (dto.TargetTestOperation, error) {
		if hostID != "0" && hostID != "" && environments != nil {
			var accepted dto.TargetTestOperation
			err := s.targets.WithActive(ctx, id, func() error {
				receipt, err := backuptarget.SyncHost(ctx, s.deps.DB, hostID, environments.ProxyJSONRequest)
				if err != nil {
					return err
				}
				response, err := backuptarget.RemoteJSON[base.ApiResponse[dto.TargetTestOperation]](ctx, environments.ProxyJSONRequest, hostID, http.MethodPost, "/api/environments/0/volumes/backups/targets/"+url.PathEscape(id)+"/test", nil)
				if err != nil {
					return err
				}
				accepted = response.Data
				return s.recordRemoteCheck(ctx, id, hostID, receipt.SetVersion, accepted.ActivityID)
			})
			return accepted, err
		}
		var accepted dto.TargetTestOperation
		err := s.targets.WithActive(ctx, id, func() error {
			var err error
			accepted, err = s.startRepositoryCheck(utils.ActivityRuntimeContext(ctx, appCtx), id)
			return err
		})
		return accepted, err
	}
	h.AttachLocalKey = func(ctx context.Context, key string) error {
		dockerClient, err := s.deps.Docker.GetClient(ctx)
		if err != nil {
			return dto.NewProblem(503, "volume.backup.engine-unavailable", err)
		}
		repository, err := s.localRusticRepositoryInternal(ctx, dockerClient, true)
		if err != nil {
			return err
		}
		if _, err = s.deps.Engine.ListSnapshots(ctx, dockerClient, repository, key); err != nil {
			return dto.NewProblem(422, "backup.target.key-mismatch", err)
		}
		return s.deps.RecoveryKeys.SetVerifiedPassword(ctx, key)
	}
	return h
}
func (s *Service) targetReferences(ctx context.Context, id string) (dto.TargetReferences, error) {
	result := dto.TargetReferences{Policies: []string{}, Backups: []string{}, Running: []string{}}
	s.policies.AdmissionLock()
	defer s.policies.AdmissionUnlock()
	err := s.deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Table("volume_backup_policies").Where("target_id = ?", id).Distinct("volume_name").Pluck("volume_name", &result.Policies).Error; e != nil {
			return e
		}
		if e := tx.Table("volume_backups").Where("destination = ?", id).Distinct("volume_name").Pluck("volume_name", &result.Backups).Error; e != nil {
			return e
		}
		return tx.Table("volume_backups").Where("destination = ? AND status = ?", id, "running").Distinct("volume_name").Pluck("volume_name", &result.Running).Error
	})
	return result, err
}
