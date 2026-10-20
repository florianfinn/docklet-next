// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant bounded remote runtime implementation; production activation is separate.

package activity

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// Startup runs before readiness in the new lifetime. Persisted admission alone
// cannot prove that a restored row was never dispatched. No Execute is replayed.
func (c *RuntimeRemoteCoordinator) Startup(ctx context.Context) error {
	c.InvalidateNegativeContinuity()
	return c.recoverInternal(ctx, nil, true)
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
		go func() { _, _ = c.Refresh(c.lifetime, ref) }()
	}
	return nil
}
