// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Migrated SQLite startup ownership, repair exclusion and atomic fault proofs.

package activity

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func b3Admit(t *testing.T, f *b1FixtureInternal, c RuntimeCommand) string {
	t.Helper()
	result, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	require.Equal(t, activitytypes.CodeRunStarted, result.Code)
	return result.ActivityID
}
func b3Receipt(t *testing.T, f *b1FixtureInternal, id string) RuntimeReceipt {
	t.Helper()
	var row RuntimeReceipt
	require.NoError(t, f.store.svc.db.First(&row, "run_id = ?", id).Error)
	return row
}
func b3Activity(t *testing.T, f *b1FixtureInternal, id string) Activity {
	t.Helper()
	var row Activity
	require.NoError(t, f.store.svc.db.First(&row, "id = ?", id).Error)
	return row
}
func b3Phase(t *testing.T, f *b1FixtureInternal, c RuntimeCommand, id, phase string) {
	t.Helper()
	identity := b1Identity(t, c, id)
	b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
		_, err := runtimeLocalAcceptTxInternal(tx, now, identity)
		if err != nil || phase == "accepted" {
			return err
		}
		_, err = runtimeLocalExecutingTxInternal(tx, now, identity)
		return err
	})
}
func b3Interrupted(t *testing.T, f *b1FixtureInternal, id string, start *time.Time) {
	t.Helper()
	row := b3Receipt(t, f, id)
	require.Equal(t, "failed", *row.ExecutorState)
	require.Equal(t, string(activitytypes.CodeRuntimeInterrupted), *row.ExecutorResultCode)
	require.Equal(t, "Runtime workflow interrupted; effects unverified", *row.ExecutorError)
	require.Equal(t, start, row.ExecutorStartedAt)
	require.NotNil(t, row.SettledAt)
	require.Nil(t, row.HubDisposition)
	activity := b3Activity(t, f, id)
	require.Equal(t, activitytypes.StatusFailed, activity.Status)
	require.Equal(t, string(activitytypes.CodeRuntimeInterrupted), activity.MessageCode)
	b2FinalCount(t, f, id, 1)
}

func TestRuntimeStartupRepairProtectsReceiptOwnership(t *testing.T) {
	f := b1Fixture(t)
	local := f.command()
	localID := b3Admit(t, f, local)
	// A copied or erased Activity scope is not receipt ownership evidence.
	require.NoError(t, f.store.svc.db.Model(&Activity{}).Where("id = ?", localID).UpdateColumns(map[string]any{"gate_scope": "", "gate_key": ""}).Error)
	remote := f.command()
	remote.Store.EnvironmentID = "example-remote"
	remote.Store.ExecutorGeneration = uuid.NewString()
	remote.Target.ID = "example-remote-stack"
	remoteID := b3Admit(t, f, remote)
	require.NoError(t, f.store.svc.db.Model(&Activity{}).Where("id = ?", remoteID).UpdateColumns(map[string]any{"gate_scope": "", "gate_key": ""}).Error)
	b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
		_, err := runtimeEvidenceTxInternal(tx, now, b1Identity(t, remote, remoteID), runtimeEvidenceInternal{State: "unknown", UnknownReason: new("executor-unavailable")})
		return err
	})
	protectedID := uuid.NewString()
	require.NoError(t, f.store.svc.db.Create(&Activity{BaseModel: database.BaseModel{ID: protectedID, CreatedAt: f.store.now},
		EnvironmentID: "0", Type: activitytypes.TypeResourceAction, Status: activitytypes.StatusQueued,
		StartedAt: f.store.now, Metadata: database.JSON{}, AffectedServices: database.JSON{}}).Error)
	beforeLocal, beforeRemote := b3Receipt(t, f, localID), b3Receipt(t, f, remoteID)
	remoteActivity := b3Activity(t, f, remoteID)
	for _, run := range []func(context.Context) (int64, error){
		func(ctx context.Context) (int64, error) { return f.store.svc.RepairStartup(ctx, protectedID) },
		func(ctx context.Context) (int64, error) {
			return f.store.svc.ResolveOrphanedQueuedActivities(ctx, protectedID)
		},
		f.store.svc.FailAbandonedActivities,
		f.store.svc.FailStaleImageUpdateChecks,
		func(ctx context.Context) (int64, error) {
			return f.store.svc.ResolveStaleAutoUpdateActivities(ctx, protectedID)
		},
	} {
		count, err := run(t.Context())
		require.NoError(t, err)
		require.Zero(t, count)
	}
	require.Equal(t, beforeLocal, b3Receipt(t, f, localID))
	require.Equal(t, beforeRemote, b3Receipt(t, f, remoteID))
	require.Equal(t, remoteActivity, b3Activity(t, f, remoteID))
	require.Equal(t, activitytypes.StatusQueued, b3Activity(t, f, protectedID).Status)
	count, err := f.store.svc.RepairStartup(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.Equal(t, activitytypes.StatusFailed, b3Activity(t, f, protectedID).Status)
	require.Equal(t, beforeLocal, b3Receipt(t, f, localID))
}

func TestRuntimeStartupLocalCohortsAndRepeatedCall(t *testing.T) {
	for _, phase := range []string{"pending", "accepted", "executing"} {
		t.Run(phase, func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			id := b3Admit(t, f, c)
			if phase != "pending" {
				b3Phase(t, f, c, id, phase)
			}
			original := b3Receipt(t, f, id)
			f.store.now = f.store.now.Add(time.Minute)
			count, err := f.service.SettleLocalStartup(t.Context(), c.Store)
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			b3Interrupted(t, f, id, original.ExecutorStartedAt)
			after := runtimeBoundarySnapshot(t, f.store)
			count, err = f.service.SettleLocalStartup(t.Context(), c.Store)
			require.NoError(t, err)
			require.Zero(t, count)
			require.Equal(t, after, runtimeBoundarySnapshot(t, f.store))
			// The original opaque key resolves without a live resolver or Activity history.
			_, err = f.store.svc.DeleteHistory(t.Context(), "0")
			require.NoError(t, err)
			f.resolver.resolve = func(context.Context, RuntimeTarget) (RuntimeResolvedTarget, error) {
				return RuntimeResolvedTarget{}, errors.New("offline")
			}
			f.store.now = f.store.now.Add(11 * time.Minute)
			retried, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			require.Equal(t, id, retried.ActivityID)
			require.Equal(t, activitytypes.StatusFailed, retried.Activity.Status)
			require.Equal(t, activitytypes.RuntimeOutcomeKnown, retried.Activity.OutcomeState)
			var activityCount int64
			require.NoError(t, f.store.svc.db.Model(&Activity{}).Where("id = ?", id).Count(&activityCount).Error)
			require.Zero(t, activityCount)
		})
	}
}

func TestRuntimeStartupRefusesUnprovedContinuityAndLiveOwner(t *testing.T) {
	f := b1Fixture(t)
	c := f.command()
	id := b3Admit(t, f, c)
	before := runtimeBoundarySnapshot(t, f.store)
	wrong := c.Store
	wrong.ManagerID = "example-other-manager"
	count, err := f.service.SettleLocalStartup(t.Context(), wrong)
	require.NoError(t, err)
	require.Zero(t, count)
	wrong = c.Store
	wrong.StoreGeneration = uuid.NewString()
	wrong.ExecutorGeneration = wrong.StoreGeneration
	count, err = f.service.SettleLocalStartup(t.Context(), wrong)
	var conflict *runtimeConflictInternal
	require.ErrorAs(t, err, &conflict)
	require.Zero(t, count)
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	require.Nil(t, b3Receipt(t, f, id).SettledAt)
	require.NoError(t, f.store.svc.db.Where("id = 1").Delete(&RuntimeStoreGeneration{}).Error)
	count, err = f.service.SettleLocalStartup(t.Context(), c.Store)
	require.ErrorIs(t, err, errRuntimeGenerationUnavailableInternal)
	require.Zero(t, count)
	// Fresh-store setup is not an absence fallback.
	var generations int64
	require.NoError(t, f.store.svc.db.Model(&RuntimeStoreGeneration{}).Count(&generations).Error)
	require.Zero(t, generations)

	entered, release := make(chan struct{}), make(chan struct{})
	live := b2Fixture(t, t.Context(), func(context.Context, RuntimeCommand, io.Writer) error { close(entered); <-release; return nil })
	defer close(release)
	liveCommand := live.command()
	liveID := b3Admit(t, live, liveCommand)
	b1Await(t, entered)
	count, err = live.service.SettleLocalStartup(t.Context(), liveCommand.Store)
	require.ErrorIs(t, err, ErrRuntimeStartupOwnersLive)
	require.Zero(t, count)
	require.Nil(t, b3Receipt(t, live, liveID).SettledAt)
}

func TestRuntimeStartupAtomicFaults(t *testing.T) {
	for _, fault := range []string{"receipt", "activity", "final-message", "commit"} {
		t.Run(fault, func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			id := b3Admit(t, f, c)
			before := runtimeBoundarySnapshot(t, f.store)
			switch fault {
			case "receipt":
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_b3_fault BEFORE UPDATE OF executor_settled_at ON runtime_receipts BEGIN SELECT RAISE(ABORT, 'fixture receipt refusal'); END").Error)
			case "activity":
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_b3_fault BEFORE UPDATE OF status ON activities WHEN NEW.status = 'failed' BEGIN SELECT RAISE(ABORT, 'fixture activity refusal'); END").Error)
			case "final-message":
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_b3_fault BEFORE INSERT ON activity_messages WHEN NEW.code = 'runtime.interrupted' BEGIN SELECT RAISE(ABORT, 'fixture final refusal'); END").Error)
			case "commit":
				b2Deferred(t, f, "NEW.code = 'runtime.interrupted'")
			}
			count, err := f.service.SettleLocalStartup(t.Context(), c.Store)
			assert.Error(t, err)
			assert.Zero(t, count)
			assert.Equal(t, before, runtimeBoundarySnapshot(t, f.store), "failed composed write must leave every table unchanged")
			genericCount, genericErr := f.store.svc.RepairStartup(t.Context())
			require.NoError(t, genericErr)
			require.Zero(t, genericCount, "generic repair cannot consume a failed runtime write")
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			trigger := "fixture_b3_fault"
			if fault == "commit" {
				trigger = "fixture_b2_fault"
			}
			require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER "+trigger).Error)
			count, err = f.service.SettleLocalStartup(t.Context(), c.Store)
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			b3Interrupted(t, f, id, nil)
		})
	}
}

func TestRuntimeStartupRemoteCohortsRemainHeld(t *testing.T) {
	f := b1Fixture(t)
	var ids []string
	for _, state := range []string{"pending", "dispatching", "accepted", "unknown"} {
		c := f.command()
		c.Store.EnvironmentID = "example-remote"
		c.Store.ExecutorGeneration = uuid.NewString()
		c.Target.ID = "example-" + state
		id := b3Admit(t, f, c)
		ids = append(ids, id)
		if state != "pending" {
			b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
				identity := b1Identity(t, c, id)
				_, err := runtimeDispatchTxInternal(tx, now, identity, false)
				if err != nil {
					return err
				}
				if state == "accepted" {
					_, err = runtimeDispatchTxInternal(tx, now, identity, true)
				}
				if state == "unknown" {
					_, err = runtimeEvidenceTxInternal(tx, now, identity, runtimeEvidenceInternal{State: "unknown", UnknownReason: new("executor-unavailable")})
				}
				return err
			})
		}
	}
	before := runtimeBoundarySnapshot(t, f.store)
	count, err := f.service.SettleLocalStartup(t.Context(), f.command().Store)
	require.NoError(t, err)
	require.Zero(t, count)
	genericCount, err := f.store.svc.RepairStartup(t.Context())
	require.NoError(t, err)
	require.Zero(t, genericCount)
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	for _, id := range ids {
		require.Nil(t, b3Receipt(t, f, id).SettledAt)
	}
}

func TestRuntimeStartupTombstoneIsNotRepairOwner(t *testing.T) {
	f := b1Fixture(t)
	c := f.command()
	runID := uuid.NewString()
	identity := b1Identity(t, c, runID)
	b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
		_, err := runtimeQueryTxInternal(tx, now, identity)
		return err
	})
	require.NoError(t, f.store.svc.db.Create(&Activity{BaseModel: database.BaseModel{ID: runID, CreatedAt: f.store.now},
		EnvironmentID: "0", Type: activitytypes.TypeResourceAction, Status: activitytypes.StatusQueued,
		StartedAt: f.store.now, Metadata: database.JSON{}, AffectedServices: database.JSON{}}).Error)
	count, err := f.store.svc.RepairStartup(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.Equal(t, activitytypes.StatusFailed, b3Activity(t, f, runID).Status)
	require.Equal(t, "not_received", b3Receipt(t, f, runID).Kind)
}

func TestRuntimeStartupRepairRequiresMigratedReceiptTable(t *testing.T) {
	f := b1Fixture(t)
	require.NoError(t, f.store.svc.db.Exec("ALTER TABLE runtime_receipts RENAME TO fixture_missing_receipts").Error)
	count, err := f.store.svc.RepairStartup(t.Context())
	require.Zero(t, count)
	require.ErrorContains(t, err, "no such table: runtime_receipts")
}
