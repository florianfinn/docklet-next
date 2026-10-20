// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Ownership-read faults clean legacy registrations without releasing runtime work.

package activity

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/settings"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func completionOwnershipFault(t *testing.T, s *ActivityService) (error, func(bool)) {
	t.Helper()
	fault := errors.New("synthetic ownership read failure")
	var active atomic.Bool
	const name = "fixture-completion-ownership"
	require.NoError(t, s.db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if _, count := tx.Statement.Dest.(*int64); tx.Statement.Table == "runtime_receipts" && count && active.Load() {
			tx.AddError(fault)
		}
	}))
	// Keep the callback list fixed while queued workers may query this fixture.
	return fault, active.Store
}

func TestRuntimeActivationLegacyCompletionOwnershipFault(t *testing.T) {
	for _, mode := range []string{"tracked-slot", "untracked-slot", "tracked-queued"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			f := runtimeFixture(t)
			fault, inject := completionOwnershipFault(t, f.svc)
			defer inject(false)
			config, err := settings.NewSettingsService(ctx, f.svc.db)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, config.Stop(context.WithoutCancel(ctx))) })
			require.NoError(t, config.SetIntSetting(ctx, maxConcurrentActivitiesSettingKey, 1))
			f.svc.limiter = newActivitySlotLimiterInternal(config)
			req := StartActivityRequest{Type: activitytypes.TypeProjectDeploy, Queue: true, DeferSlot: mode == "tracked-queued"}
			row, err := f.svc.StartActivity(ctx, req)
			require.NoError(t, err)
			var work context.Context
			if mode != "untracked-slot" {
				work = f.svc.Track(ctx, row.ID)
			}
			heldCapacity := func() int {
				f.svc.limiter.mu.Lock()
				defer f.svc.limiter.mu.Unlock()
				return f.svc.limiter.stateForLockedInternal(row.EnvironmentID).held
			}
			require.Equal(t, map[bool]int{false: 1, true: 0}[mode == "tracked-queued"], heldCapacity())
			inject(true)
			_, err = f.svc.CompleteActivity(ctx, row.ID, activitytypes.StatusSuccess, "Finished", nil)
			inject(false)
			require.ErrorIs(t, err, fault)
			f.svc.slotMu.Lock()
			_, held := f.svc.slotReleases[row.ID]
			f.svc.slotMu.Unlock()
			require.False(t, held, "known legacy completion must release capacity even when ownership cannot be read")
			require.Zero(t, heldCapacity(), "the actual limiter must recover its capacity")
			release, available := f.svc.limiter.tryAcquireInternal(ctx, row.EnvironmentID).Get()
			require.True(t, available, "another legacy operation can acquire the freed slot")
			release()
			f.svc.runningMu.Lock()
			_, tracked := f.svc.running[row.ID]
			f.svc.runningMu.Unlock()
			require.False(t, tracked, "known legacy cancellation registration must not leak")
			if work != nil {
				require.ErrorIs(t, work.Err(), context.Canceled)
			}
			require.Equal(t, row.Status, matrixRow(t, f.svc.db.DB, row.ID).Status, "failed lookup never fabricates terminal persistence")
		})
	}
}

func TestRuntimeActivationRuntimeCompletionOwnershipFault(t *testing.T) {
	for _, mode := range []struct{ queued, bound bool }{{false, true}, {true, true}, {false, false}, {true, false}} {
		queued := mode.queued
		t.Run(map[bool]string{false: "executing", true: "queued"}[queued]+map[bool]string{false: "-unbound", true: "-bound"}[mode.bound], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			probe := slotsNewProbe(t)
			f := slotsFixture(t, ctx, new(1), func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error { return probe.run(ctx) })
			slotsCleanup(t, f, probe)
			if mode.bound {
				require.NoError(t, f.service.BindActivityOwner())
			}
			var unblock func()
			if queued {
				var err error
				unblock, err = f.store.svc.limiter.acquireInternal(ctx, "0")
				require.NoError(t, err)
				defer unblock()
			}
			fault, inject := completionOwnershipFault(t, f.store.svc)
			defer inject(false)
			c := f.command()
			admitted, err := f.service.Admit(ctx, c)
			require.NoError(t, err)
			if !queued {
				b1Await(t, probe.entered)
			}
			owner := b2Owner(t, f, admitted.ActivityID)
			before := matrixRow(t, f.store.svc.db.DB, admitted.ActivityID)
			inject(true)
			_, err = f.store.svc.CompleteActivity(ctx, admitted.ActivityID, activitytypes.StatusSuccess, "False completion", nil)
			inject(false)
			require.ErrorIs(t, err, fault)
			require.NoError(t, owner.ctx.Err(), "generic completion cannot cancel the runtime owner")
			require.Equal(t, before, matrixRow(t, f.store.svc.db.DB, admitted.ActivityID))
			f.store.svc.slotMu.Lock()
			slot, held := f.store.svc.slotReleases[admitted.ActivityID]
			f.store.svc.slotMu.Unlock()
			require.Equal(t, !queued, held, "runtime capacity remains held until joined terminal persistence")
			if held {
				require.True(t, slot.runtime)
			}
			conflicting := c
			conflicting.Key, conflicting.Operation, conflicting.Options = uuid.NewString(), RuntimeDownProject, RuntimeOptions{}
			_, err = f.service.Admit(ctx, conflicting)
			var busy *GateConflict
			require.ErrorAs(t, err, &busy)
			if queued {
				unblock()
				b1Await(t, probe.entered)
			}
			probe.unblock()
			snapshot, err := b2Wait(t, f, c, admitted.ActivityID)
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusSuccess, snapshot.Status)
			require.EqualValues(t, 1, probe.calls.Load())
			require.False(t, slotsRegistered(f, admitted.ActivityID))
			slotsHeld(t, f, 0)
		})
	}
}
