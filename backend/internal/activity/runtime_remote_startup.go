// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Bounded remote runtime implementation, active in production since #125.

package activity

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Startup runs before readiness in the new lifetime. Persisted admission alone
// cannot prove that a restored row was never dispatched. No Execute is replayed.
func (c *RuntimeRemoteCoordinator) Startup(ctx context.Context) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if err := c.lifetime.Err(); err != nil {
		return errors.Join(err, c.closeInternal(ctx))
	}
	// Join even after an earlier Close timed out; an old task must never write
	// into a resumed lifetime or remove its replacement's registration.
	if err := c.closeInternal(ctx); err != nil {
		return err
	}
	c.InvalidateNegativeContinuity()
	c.mu.Lock()
	c.paused = false
	c.mu.Unlock()
	if err := c.recoverInternal(ctx, nil, true); err != nil {
		return errors.Join(err, c.closeInternal(ctx))
	}
	if err := errors.Join(ctx.Err(), c.lifetime.Err()); err != nil {
		return errors.Join(err, c.closeInternal(ctx))
	}
	return nil
}
func (c *RuntimeRemoteCoordinator) RecoverTarget(ctx context.Context, store RuntimeStoreBinding) error {
	return c.recoverInternal(ctx, &store, false)
}
func (c *RuntimeRemoteCoordinator) recoverInternal(ctx context.Context, only *RuntimeStoreBinding, startup bool) error {
	s := c.service
	s.activity.runningMu.Lock()
	var rows []RuntimeReceipt
	err := s.transactionInternal(ctx, func(tx *gorm.DB, _ time.Time) error {
		query := tx.Where("kind = 'receipt' AND environment_id <> '0' AND settled_at IS NULL")
		if only != nil {
			query = query.Where("environment_id = ? AND registration_id = ? AND executor_generation = ?", only.EnvironmentID, only.RegistrationID, only.ExecutorGeneration)
		}
		return query.Order("run_id").Find(&rows).Error
	})
	s.activity.runningMu.Unlock()
	if err != nil {
		return err
	}
	for _, row := range rows {
		ref := RuntimeRunReference{Store: runtimeStoreFromRowInternal(&row), RunID: row.RunID}
		if *row.DispatchState == "pending" {
			if startup {
				if _, err := s.remoteDispositionInternal(ctx, ref, runtimeDispositionInternal{Disposition: "interrupted", By: "runtime-reconciler", Reason: "Hub workflow interrupted; effects unverified"}); err != nil {
					return err
				}
			}
			continue
		}
		if _, err := s.remoteUnknownInternal(ctx, ref, "process-continuity"); err != nil {
			return err
		}
		if err := c.scheduleRefreshInternal(ctx, ref); err != nil {
			return err
		}
	}
	return nil
}

// RecoverHeldTarget schedules only transport or continuity holds for this exact
// executor identity. Evidence and administrator dispositions are never rewritten.
func (c *RuntimeRemoteCoordinator) RecoverHeldTarget(ctx context.Context, store RuntimeStoreBinding) error {
	s := c.service
	s.activity.runningMu.Lock()
	var rows []RuntimeReceipt
	err := s.transactionInternal(ctx, func(tx *gorm.DB, _ time.Time) error {
		return heldReceiptsInternal(tx, store.EnvironmentID).
			Where("registration_id = ? AND executor_generation = ? AND manager_id = ? AND store_generation = ?", store.RegistrationID, store.ExecutorGeneration, store.ManagerID, store.StoreGeneration).
			Order("run_id").Find(&rows).Error
	})
	s.activity.runningMu.Unlock()
	if err != nil {
		return err
	}
	// One run that cannot refresh now does not delay the other held runs of this host.
	var errs []error
	for _, row := range rows {
		ref := RuntimeRunReference{Store: runtimeStoreFromRowInternal(&row), RunID: row.RunID}
		if _, _, err := c.prepareRefreshInternal(ctx, ref, false, true, true); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// HasHeldTarget reports whether the environment has any held run RecoverHeldTarget could select, so a
// health tick contacts the agent for its identity only when there is something to recover.
func (c *RuntimeRemoteCoordinator) HasHeldTarget(ctx context.Context, environmentID string) (bool, error) {
	var count int64
	err := heldReceiptsInternal(c.service.activity.db.WithContext(ctx), environmentID).Model(&RuntimeReceipt{}).Count(&count).Error
	return count > 0, err
}

func heldReceiptsInternal(tx *gorm.DB, environmentID string) *gorm.DB {
	return tx.Where("kind = 'receipt' AND environment_id <> '0' AND settled_at IS NULL AND hub_disposition IS NULL AND executor_state = 'unknown' AND unknown_reason IN ?", []string{"transport", "process-continuity"}).
		Where("environment_id = ?", environmentID)
}
