// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Atomic production environment removal and runtime exclusion.

package activity

import (
	"context"
	"errors"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
)

// RemoveEnvironment never consults an offline executor. Every held registration
// for the environment must already have a durable disposition before deletion.
func (s *RuntimeService) RemoveEnvironment(ctx context.Context, environment string, remove func(*gorm.DB) error) error {
	if err := s.checkInternal(); err != nil {
		return err
	}
	if environment == "0" || environment == "" || remove == nil {
		return errRuntimeInvalidInternal
	}
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	if s.localExecution == nil {
		return errRuntimeInvalidInternal
	}
	return s.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
		if s.intakePaused || s.storagePaused {
			return ErrRuntimeNotReady
		}
		local := s.localExecution.Store
		store := RuntimeStoreBinding{EnvironmentID: environment, RegistrationID: environment, ManagerID: local.ManagerID, StoreGeneration: local.StoreGeneration, ExecutorGeneration: "unregistered"}
		if err := runtimeCheckStoreTxInternal(tx, runtimeIdentityInternal{StoreGeneration: store.StoreGeneration, ManagerID: store.ManagerID, RegistrationID: store.RegistrationID, EnvironmentID: environment, ExecutorGeneration: store.ExecutorGeneration, RequestDigest: "removal"}); err != nil {
			return err
		}
		var held int64
		if err := tx.Model(&RuntimeReceipt{}).Where("environment_id = ? AND kind = 'receipt' AND settled_at IS NULL", environment).Count(&held).Error; err != nil {
			return err
		}
		if held != 0 {
			return common.NewAPIError("Runtime recovery required before environment removal", common.APIErrorCode(activitytypes.CodeRunBusy), 409)
		}
		var rows []RuntimeReceipt
		if err := tx.Where("environment_id = ? AND kind = 'receipt'", environment).Find(&rows).Error; err != nil {
			return err
		}
		stores := map[string]RuntimeStoreBinding{environment: store}
		for i := range rows {
			stores[rows[i].RegistrationID] = runtimeStoreFromRowInternal(&rows[i])
		}
		for _, binding := range stores {
			var existing RuntimeRemovedTarget
			err := tx.First(&existing, "registration_id = ?", binding.RegistrationID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := runtimeReceiverCreateTxInternal(tx, &RuntimeRemovedTarget{RegistrationID: binding.RegistrationID, ManagerID: binding.ManagerID, EnvironmentID: environment, StoreGeneration: local.StoreGeneration, ExecutorGeneration: binding.ExecutorGeneration, RemovedAt: now}); err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
		}
		if err := tx.Model(&RuntimeReceipt{}).Where("environment_id = ? AND final_write_at < ?", environment, now).UpdateColumn("final_write_at", now).Error; err != nil {
			return err
		}
		return remove(tx)
	})
}
