// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Bounded remote runtime implementation, active in production since #125.

package activity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
)

var ErrRuntimeRecoveryDenied = errors.New("runtime recovery permission denied")
var ErrRuntimeRecoveryInProgress = errors.New("runtime recovery in progress")
var ErrRuntimeTargetRemoved = common.NewAPIError("Runtime target removed", common.APIErrorCode(activitytypes.CodeRuntimeTargetRemoved), 410)

type RuntimeRemovedTarget struct {
	RegistrationID     string    `gorm:"column:registration_id;primaryKey;not null"`
	ManagerID          string    `gorm:"column:manager_id;not null"`
	EnvironmentID      string    `gorm:"column:environment_id;not null"`
	StoreGeneration    string    `gorm:"column:store_generation;not null"`
	ExecutorGeneration string    `gorm:"column:executor_generation;not null"`
	RemovedAt          time.Time `gorm:"column:removed_at;not null"`
}

func (RuntimeRemovedTarget) TableName() string { return "runtime_removed_targets" }
func runtimeTargetPresentTxInternal(tx *gorm.DB, store RuntimeStoreBinding) error {
	var removed RuntimeRemovedTarget
	err := tx.First(&removed, "registration_id = ?", store.RegistrationID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return ErrRuntimeTargetRemoved
}
func runtimeStoreFromRowInternal(row *RuntimeReceipt) RuntimeStoreBinding {
	return RuntimeStoreBinding{StoreGeneration: row.StoreGeneration, ManagerID: row.ManagerID, RegistrationID: row.RegistrationID, EnvironmentID: row.EnvironmentID, ExecutorGeneration: row.ExecutorGeneration}
}

// Release requires effective global administration and activity access from the
// authenticated context. The reason/acknowledgment records operator-controlled
// executor shutdown and actual-state inspection, never mechanical effect proof.
func (c *RuntimeRemoteCoordinator) Release(ctx context.Context, ref RuntimeRunReference, reason string, acknowledged bool) (activitytypes.RuntimeSnapshot, error) {
	ps, _ := middleware.PermissionsFromContext(ctx)
	actor, ok := user.CurrentUserFromContext(ctx)
	if !ok || actor == nil || !ps.IsGlobalAdmin() || !ps.Allows(authz.PermActivitiesRead, ref.Store.EnvironmentID) || !ps.Allows(authz.PermActivitiesCancel, ref.Store.EnvironmentID) {
		return activitytypes.RuntimeSnapshot{}, ErrRuntimeRecoveryDenied
	}
	reason = strings.TrimSpace(reason)
	if !acknowledged || !runtimeTextInternal(reason, 2048) || !runtimeTextInternal(actor.ID, 256) {
		return activitytypes.RuntimeSnapshot{}, errRuntimeInvalidInternal
	}
	c.mu.Lock()
	if c.recovering[ref.RunID] {
		c.mu.Unlock()
		return activitytypes.RuntimeSnapshot{}, ErrRuntimeRecoveryInProgress
	}
	c.recovering[ref.RunID] = true
	var tasks []*runtimeRemoteTaskInternal
	if task := c.dispatch[ref.RunID]; task != nil {
		task.cancel()
		tasks = append(tasks, task)
	}
	if task := c.queries[ref.RunID]; task != nil {
		task.cancel()
		tasks = append(tasks, task)
	}
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.recovering, ref.RunID); c.mu.Unlock() }()
	for _, task := range tasks {
		select {
		case <-ctx.Done():
			return activitytypes.RuntimeSnapshot{}, ctx.Err()
		case <-task.done:
		}
	}
	s := c.service
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	lock := s.activity.publishLockInternal(ref.RunID)
	lock.Lock()
	defer lock.Unlock()
	var snapshot activitytypes.RuntimeSnapshot
	var final *ActivityMessage
	err := s.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
		row, err := runtimeReferenceRowTxInternal(tx, ref)
		if err != nil {
			return err
		}
		if row.HubDisposition != nil && *row.HubDisposition == "released" {
			snapshot, err = runtimeSnapshotTxInternal(tx, row)
			return err
		}
		if (row.EnvironmentID == "0" && s.owners[row.RunID] != nil) || row.ExecutorSettledAt != nil || runtimeKnowledgeInternal(row).OutcomeState != activitytypes.RuntimeOutcomeUnknown || (row.EnvironmentID != "0" && *row.DispatchState == "pending") {
			return &runtimeConflictInternal{}
		}
		snapshot, final, err = runtimeHubDispositionTxInternal(tx, now, row, runtimeDispositionInternal{Disposition: "released", By: actor.ID, Reason: reason, ReleaseAcknowledged: true})
		return err
	})
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	if final != nil {
		s.activity.publishMessageInternal(snapshot.EnvironmentID, snapshot.Type, activityMessageToDTOInternal(final))
		s.activity.publishActivityInternal(snapshot.Activity)
	}
	return snapshot, nil
}

// RemovalGuard returns durable recovery references, without modifying the target
// or hiding unknown outcomes. MarkRemoved rechecks the predicate atomically.
type RuntimeRemovalBlocked struct{ Runs []RuntimeRunReference }

func (e *RuntimeRemovalBlocked) Error() string {
	return "runtime recovery required before target removal"
}
func (s *RuntimeService) RemovalGuard(ctx context.Context, store RuntimeStoreBinding) error {
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	return s.transactionInternal(ctx, func(tx *gorm.DB, _ time.Time) error { return runtimeRemovalGuardTxInternal(tx, store) })
}
func runtimeRemovalGuardTxInternal(tx *gorm.DB, store RuntimeStoreBinding) error {
	if err := runtimeCheckStoreTxInternal(tx, runtimeIdentityInternal{StoreGeneration: store.StoreGeneration, ManagerID: store.ManagerID, RegistrationID: store.RegistrationID, EnvironmentID: store.EnvironmentID, ExecutorGeneration: store.ExecutorGeneration, RequestDigest: "removal"}); err != nil {
		return err
	}
	var rows []RuntimeReceipt
	if err := tx.Where("kind = 'receipt' AND registration_id = ? AND settled_at IS NULL", store.RegistrationID).Order("run_id").Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	blocked := &RuntimeRemovalBlocked{}
	for _, row := range rows {
		blocked.Runs = append(blocked.Runs, RuntimeRunReference{Store: runtimeStoreFromRowInternal(&row), RunID: row.RunID})
	}
	return blocked
}
func (s *RuntimeService) MarkRemoved(ctx context.Context, store RuntimeStoreBinding) error {
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	return s.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
		if err := runtimeRemovalGuardTxInternal(tx, store); err != nil {
			return err
		}
		var existing RuntimeRemovedTarget
		err := tx.First(&existing, "registration_id = ?", store.RegistrationID).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := runtimeReceiverCreateTxInternal(tx, &RuntimeRemovedTarget{RegistrationID: store.RegistrationID, ManagerID: store.ManagerID, EnvironmentID: store.EnvironmentID, StoreGeneration: store.StoreGeneration, ExecutorGeneration: store.ExecutorGeneration, RemovedAt: now}); err != nil {
			return err
		}
		return tx.Model(&RuntimeReceipt{}).Where("registration_id = ? AND final_write_at < ?", store.RegistrationID, now).UpdateColumn("final_write_at", now).Error
	})
}

// RecoveryDetail exposes late executor evidence separately from the immutable
// manual Activity disposition. Production routes must authorize the reference.
type RuntimeRecoveryDetail struct {
	Activity           activitytypes.RuntimeSnapshot
	ExecutorState      string
	ExecutorResultCode *string
	ExecutorError      *string
	ExecutorSettledAt  *time.Time
	Disposition        *string
	DispositionReason  *string
}

func (s *RuntimeService) RecoveryDetail(ctx context.Context, ref RuntimeRunReference) (RuntimeRecoveryDetail, error) {
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	var out RuntimeRecoveryDetail
	err := s.transactionInternal(ctx, func(tx *gorm.DB, _ time.Time) error {
		row, err := runtimeReferenceRowTxInternal(tx, ref)
		if err != nil {
			return err
		}
		snapshot, err := runtimeSnapshotTxInternal(tx, row)
		if err != nil {
			return err
		}
		out = RuntimeRecoveryDetail{Activity: snapshot, ExecutorState: *row.ExecutorState, ExecutorResultCode: row.ExecutorResultCode, ExecutorError: row.ExecutorError, ExecutorSettledAt: row.ExecutorSettledAt, Disposition: row.HubDisposition, DispositionReason: row.HubDispositionReason}
		return nil
	})
	if err != nil {
		return RuntimeRecoveryDetail{}, err
	}
	return out, nil
}
