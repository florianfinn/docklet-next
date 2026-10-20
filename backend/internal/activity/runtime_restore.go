// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Runtime ownership joins the existing production restore boundary.

package activity

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrRuntimeNotReady = errors.New("runtime intake is paused")

// PrepareRuntimeRestore runs under the existing maintenance gate before any
// pool is suspended. It joins runtime work and transports; no held remote run
// becomes terminal merely because its hub-side transport closed.
func (a *ActivityService) PrepareRuntimeRestore(ctx context.Context) error {
	s := a.runtime
	if s == nil {
		return nil
	}
	s.restoreMu.Lock()
	defer s.restoreMu.Unlock()
	if a.runtimeHTTP != nil {
		a.runtimeHTTP.SetReady(false)
	}
	a.runningMu.Lock()
	s.intakePaused = true
	for _, owner := range s.owners {
		owner.cancel(context.Canceled)
	}
	a.runningMu.Unlock()
	if s.remote != nil {
		if err := s.remote.Close(ctx); err != nil {
			return err
		}
	}
	if err := s.JoinLocal(ctx); err != nil {
		return err
	}
	a.runningMu.Lock()
	s.storagePaused = true
	a.runningMu.Unlock()
	return nil
}

// FinishRuntimeRestore executes after both receipt-preserving repair helpers
// and durable session revocation, before scheduler/intake resume. A restored
// store cannot advertise the prior executor generation as continuous evidence.
func (a *ActivityService) FinishRuntimeRestore(ctx context.Context) error {
	s := a.runtime
	if s == nil {
		return nil
	}
	s.restoreMu.Lock()
	defer s.restoreMu.Unlock()
	if s.receiver != nil {
		s.receiver.binding.Lock()
		defer s.receiver.binding.Unlock()
	}
	var restored RuntimeStoreGeneration
	if err := a.db.WithContext(ctx).First(&restored, "id = 1").Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return &RuntimeStoreError{Cause: err}
		}
		var receipts int64
		if err := a.db.WithContext(ctx).Model(&RuntimeReceipt{}).Count(&receipts).Error; err != nil {
			return &RuntimeStoreError{Cause: err}
		}
		if receipts != 0 {
			return &RuntimeStoreError{Cause: errors.New("restored runtime generation missing for retained receipts")}
		}
		generation, err := s.InitializeGeneration(ctx)
		if err != nil {
			return err
		}
		restored.Generation = generation
	}
	var manager struct{ Value string }
	read := a.db.WithContext(ctx).Table("settings").Select("value").Where("key = ?", "instanceId").Take(&manager)
	if read.Error != nil {
		return &RuntimeStoreError{Cause: read.Error}
	}
	if read.RowsAffected != 1 || !runtimeTextInternal(manager.Value, 256) {
		return &RuntimeStoreError{Cause: errors.New("restored runtime manager identity unavailable")}
	}
	binding, err := s.LocalBinding(ctx, restored.Generation, manager.Value)
	if err != nil {
		return err
	}
	a.runningMu.Lock()
	if len(s.owners) != 0 {
		a.runningMu.Unlock()
		return ErrRuntimeStartupOwnersLive
	}
	s.localExecution.Store = binding
	a.runningMu.Unlock()
	if err := s.InitializeLocalStartup(ctx, binding); err != nil {
		return err
	}
	generation := uuid.NewString()
	a.runningMu.Lock()
	err = s.transactionInternal(ctx, func(tx *gorm.DB, _ time.Time) error {
		changed := tx.Model(&RuntimeStoreGeneration{}).Where("id = 1 AND generation = ?", restored.Generation).UpdateColumn("generation", generation)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return &runtimeConflictInternal{}
		}
		if err := tx.Model(&RuntimeReceipt{}).Where("store_generation = ?", restored.Generation).UpdateColumn("store_generation", generation).Error; err != nil {
			return err
		}
		return tx.Model(&RuntimeRemovedTarget{}).Where("store_generation = ?", restored.Generation).UpdateColumn("store_generation", generation).Error
	})
	if err == nil {
		binding.StoreGeneration, binding.ExecutorGeneration = generation, generation
		s.localExecution.Store = binding
		if s.receiver != nil {
			s.receiver.store = binding
		}
	}
	a.runningMu.Unlock()
	if err != nil {
		return err
	}
	if s.remote != nil {
		if err := s.remote.Startup(ctx); err != nil {
			return err
		}
	}
	a.runningMu.Lock()
	s.storagePaused = false
	a.runningMu.Unlock()
	return nil
}

// ResumeRuntimeIntake restores an aborted pre-replacement lifetime before
// opening readiness. Successful replacement already recovered in Finish.
// Any recovery failure leaves both storage maintenance and intake closed.
func (a *ActivityService) ResumeRuntimeIntake(ctx context.Context) error {
	s := a.runtime
	if s == nil {
		return nil
	}
	s.restoreMu.Lock()
	defer s.restoreMu.Unlock()
	a.runningMu.Lock()
	paused, storagePaused := s.intakePaused, s.storagePaused
	a.runningMu.Unlock()
	if !paused {
		return nil
	}
	if s.localExecution != nil {
		if err := s.localExecution.ApplicationContext.Err(); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if storagePaused && s.remote != nil {
		if err := s.remote.Startup(ctx); err != nil {
			return err
		}
	}
	a.runningMu.Lock()
	s.storagePaused = false
	s.intakePaused = false
	a.runningMu.Unlock()
	if a.runtimeHTTP != nil {
		a.runtimeHTTP.SetReady(true)
	}
	return nil
}
