// Source: docklet-next/docs/design/runtime-control.md
// Revision: c641dc24b3f364ac5e4ec03bc9c5dfe2a10d5f61a6bcba825785f9dc351a9e50
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Store-clock compact retention and actual history/output independence proofs.

package activity

import (
	"context"
	"fmt"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func runtimeSettle(t *testing.T, f *runtimeFixtureInternal, request runtimeAdmissionInternal) *RuntimeReceipt {
	t.Helper()
	var row *RuntimeReceipt
	require.NoError(t, f.tx(func(tx *gorm.DB, now time.Time) error {
		var err error
		row, err = runtimeEvidenceTxInternal(tx, now, request.Identity,
			runtimeEvidenceInternal{State: "success", SettledAt: new(now.Add(7 * time.Hour))})
		return err
	}))
	return row
}

func runtimeNegative(t *testing.T, f *runtimeFixtureInternal, id runtimeIdentityInternal) *RuntimeReceipt {
	t.Helper()
	var row *RuntimeReceipt
	require.NoError(t, f.tx(func(tx *gorm.DB, now time.Time) error {
		var err error
		row, err = runtimeQueryTxInternal(tx, now, id)
		return err
	}))
	return row
}

func TestRuntimeRetentionExactFloorAndLaterJoin(t *testing.T) {
	for _, negative := range []bool{false, true} {
		t.Run(fmt.Sprint("negative=", negative), func(t *testing.T) {
			f := runtimeFixture(t)
			request := f.request()
			if negative {
				runtimeNegative(t, f, request.Identity)
			} else {
				_, err := f.admit(request)
				require.NoError(t, err)
				runtimeSettle(t, f, request)
			}
			written := runtimeTimeInternal(f.now)
			for _, delta := range []time.Duration{24*time.Hour - time.Microsecond, 24 * time.Hour, 24*time.Hour + time.Microsecond} {
				f.now = written.Add(delta)
				count, err := f.svc.pruneRuntimeInternal(t.Context(), 1)
				require.NoError(t, err)
				want := int64(0)
				if delta == 24*time.Hour {
					want = 1
				}
				require.Equal(t, want, count)
			}
			runtimeCount(t, f.svc.db.DB, &RuntimeReceipt{}, 0)
			runtimeCount(t, f.svc.db.DB, &RuntimeKeyBinding{}, 0)
			if !negative {
				runtimeCount(t, f.svc.db.DB, &Activity{}, 1)
			}
			runtimeCount(t, f.svc.db.DB, &RuntimeStoreGeneration{}, 1)
		})
	}
	f := runtimeFixture(t)
	request := f.request()
	_, err := f.admit(request)
	require.NoError(t, err)
	runtimeSettle(t, f, request)
	settled := f.now
	f.now = settled.Add(23 * time.Hour)
	joinedKey := runtimeBindingInternal{Key: uuid.NewString(), CallerID: "example-joiner"}
	joined, err := f.join(request.Identity, joinedKey)
	require.NoError(t, err)
	joinedAt := joined.FinalWriteAt
	f.now = settled.Add(24 * time.Hour)
	count, err := f.svc.pruneRuntimeInternal(t.Context(), 100)
	require.NoError(t, err)
	require.Zero(t, count)
	row, err := f.lookup(request)
	require.NoError(t, err)
	require.Equal(t, joined, row)
	runtimeCount(t, f.svc.db.DB, &RuntimeKeyBinding{}, 2)
	f.now = joinedAt.Add(24*time.Hour - time.Microsecond)
	count, err = f.svc.pruneRuntimeInternal(t.Context(), 100)
	require.NoError(t, err)
	require.Zero(t, count)
	f.now = joinedAt.Add(24 * time.Hour)
	count, err = f.svc.pruneRuntimeInternal(t.Context(), 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	runtimeCount(t, f.svc.db.DB, &RuntimeKeyBinding{}, 0)
}

func TestRuntimeRetentionAncientActiveUnknownAndAcknowledgedDisposition(t *testing.T) {
	f := runtimeFixture(t)
	active, unknown, disposed := f.request(), f.request(), f.request()
	for _, request := range []runtimeAdmissionInternal{active, unknown, disposed} {
		_, err := f.admit(request)
		require.NoError(t, err)
	}
	for _, request := range []runtimeAdmissionInternal{unknown, disposed} {
		require.NoError(t, f.tx(func(tx *gorm.DB, now time.Time) error {
			_, err := runtimeEvidenceTxInternal(tx, now, request.Identity,
				runtimeEvidenceInternal{State: "unknown", UnknownReason: new("executor-unavailable")})
			return err
		}))
	}
	require.NoError(t, f.tx(func(tx *gorm.DB, now time.Time) error {
		_, err := runtimeDispositionTxInternal(tx, now, disposed.Identity,
			runtimeDispositionInternal{Disposition: "released", By: "example-admin", Reason: "Stopped and inspected", ReleaseAcknowledged: true})
		return err
	}))
	f.now = f.now.Add(365 * 24 * time.Hour)
	count, err := f.svc.pruneRuntimeInternal(t.Context(), 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	for _, request := range []runtimeAdmissionInternal{active, unknown} {
		row, err := f.lookup(request)
		require.NoError(t, err)
		require.Nil(t, row.SettledAt)
	}
	runtimeCount(t, f.svc.db.DB, &RuntimeReceipt{}, 2)
	runtimeCount(t, f.svc.db.DB, &RuntimeKeyBinding{}, 2)
	count, err = f.svc.pruneRuntimeInternal(t.Context(), 100)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestRuntimeRetentionStableOrderLimitAndSingleClock(t *testing.T) {
	f := runtimeFixture(t)
	base := runtimeTimeInternal(f.now)
	var ids []string
	for i := 1; i <= 4; i++ {
		id := f.request().Identity
		id.RunID = fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		ids = append(ids, id.RunID)
		// IDs 2/3/4 tie; ID 1 is newer and must lose the time ordering.
		f.now = base
		if i == 1 {
			f.now = base.Add(time.Second)
		}
		runtimeNegative(t, f, id)
	}
	f.now = base.Add(25 * time.Hour)
	clockCalls := 0
	f.svc.clock = func() time.Time { clockCalls++; return f.now }
	for _, limit := range []int{0, -1} {
		count, err := f.svc.pruneRuntimeInternal(t.Context(), limit)
		require.NoError(t, err)
		require.Zero(t, count)
	}
	before := clockCalls
	count, err := f.svc.pruneRuntimeInternal(t.Context(), 2)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	require.Equal(t, before+1, clockCalls)
	var remaining []string
	require.NoError(t, f.svc.db.Model(&RuntimeReceipt{}).Order("run_id").Pluck("run_id", &remaining).Error)
	require.Equal(t, []string{ids[0], ids[3]}, remaining)
	count, err = f.svc.pruneRuntimeInternal(t.Context(), 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.NoError(t, f.svc.db.Model(&RuntimeReceipt{}).Pluck("run_id", &remaining).Error)
	require.Equal(t, []string{ids[0]}, remaining)
	count, err = f.svc.pruneRuntimeInternal(t.Context(), 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	count, err = f.svc.pruneRuntimeInternal(t.Context(), 100)
	require.NoError(t, err)
	require.Zero(t, count)
	// Even a huge requested limit cannot turn the query into an unbounded batch.
	for i := 0; i < runtimePruneCapInternal+1; i++ {
		f.now = base
		runtimeNegative(t, f, f.request().Identity)
	}
	f.now = base.Add(25 * time.Hour)
	count, err = f.svc.pruneRuntimeInternal(t.Context(), int(^uint(0)>>1))
	require.NoError(t, err)
	require.EqualValues(t, runtimePruneCapInternal, count)
	runtimeCount(t, f.svc.db.DB, &RuntimeReceipt{}, 1)
}

func TestRuntimeRetentionDeleteFailureRollsBackWholeBatch(t *testing.T) {
	for _, failing := range []string{"runtime_key_bindings", "runtime_receipts"} {
		t.Run(failing, func(t *testing.T) {
			f := runtimeFixture(t)
			for i := 0; i < 2; i++ {
				request := f.request()
				_, err := f.admit(request)
				require.NoError(t, err)
				runtimeSettle(t, f, request)
			}
			f.now = f.now.Add(25 * time.Hour)
			require.NoError(t, f.svc.db.Exec("CREATE TRIGGER reject_prune BEFORE DELETE ON "+failing+" BEGIN SELECT RAISE(ABORT, 'fixture delete rejected'); END").Error)
			count, err := f.svc.pruneRuntimeInternal(t.Context(), 100)
			require.ErrorContains(t, err, "fixture delete rejected")
			require.Zero(t, count)
			runtimeCount(t, f.svc.db.DB, &RuntimeReceipt{}, 2)
			runtimeCount(t, f.svc.db.DB, &RuntimeKeyBinding{}, 2)
			runtimeCount(t, f.svc.db.DB, &Activity{}, 2)
			require.NoError(t, f.svc.db.Exec("DROP TRIGGER reject_prune").Error)
			count, err = f.svc.pruneRuntimeInternal(t.Context(), 100)
			require.NoError(t, err)
			require.EqualValues(t, 2, count)
		})
	}
}

func TestRuntimeRetentionClockJumpsDoNotLatchRefusal(t *testing.T) {
	f := runtimeFixture(t)
	request := f.request()
	original, err := f.admit(request)
	require.NoError(t, err)
	runtimeSettle(t, f, request)
	f.now = f.now.Add(-time.Hour)
	row, err := f.join(request.Identity, runtimeBindingInternal{Key: uuid.NewString(), CallerID: "example-joiner"})
	require.NoError(t, err)
	require.Equal(t, original.FinalWriteAt, row.FinalWriteAt)
	count, err := f.svc.pruneRuntimeInternal(t.Context(), 100)
	require.NoError(t, err)
	require.Zero(t, count)
	f.now = f.now.Add(50 * time.Hour)
	count, err = f.svc.pruneRuntimeInternal(t.Context(), 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	f.now = f.now.Add(-100 * time.Hour)
	fresh := f.request()
	row, err = f.admit(fresh)
	require.NoError(t, err)
	require.Equal(t, fresh.Identity.RunID, row.RunID)
	require.Equal(t, runtimeTimeInternal(f.now), *row.AdmittedAt)
}

func TestRuntimeRetentionHistoryAndOutputRemainIndependent(t *testing.T) {
	f := runtimeFixture(t)
	settings, err := newSettingsServiceForTestInternal(t, t.Context(), f.svc.db)
	require.NoError(t, err)
	f.svc.settings = settings
	var requests []runtimeAdmissionInternal
	for i := 0; i < 103; i++ {
		request := f.request()
		requests = append(requests, request)
		_, err := f.admit(request)
		require.NoError(t, err)
		require.NoError(t, settings.SetBoolSetting(t.Context(), "keepActionOutput", i != 0))
		ctx := context.WithValue(t.Context(), gateRunContextKey{}, request.Identity.RunID)
		line := activitytypes.OutputLine{Timestamp: runtimeTimeInternal(f.now), Stream: "stdout", Text: "Example output"}
		_, err = f.svc.AppendMessages(ctx, request.Identity.RunID, []AppendActivityMessageRequest{{Output: &line}})
		require.NoError(t, err)
		detail, err := f.svc.GetActivityDetail(t.Context(), "0", request.Identity.RunID, 10)
		require.NoError(t, err)
		if i == 0 {
			require.Empty(t, detail.Output)
		} else {
			require.Equal(t, []activitytypes.OutputLine{line}, detail.Output)
		}
		_, err = f.svc.CompleteActivity(ctx, request.Identity.RunID, activitytypes.StatusSuccess, "Done", nil)
		require.NoError(t, err)
		runtimeSettle(t, f, request)
		f.now = f.now.Add(time.Second)
	}
	count, err := f.svc.PruneHistory(t.Context(), 100)
	require.NoError(t, err)
	require.EqualValues(t, 3, count)
	runtimeCount(t, f.svc.db.DB, &Activity{}, 100)
	runtimeCount(t, f.svc.db.DB, &RuntimeReceipt{}, 103)
	runtimeCount(t, f.svc.db.DB, &RuntimeKeyBinding{}, 103)
	f.now = f.now.Add(11 * time.Minute)
	for _, request := range requests[:3] {
		_, err := f.svc.GetActivityDetail(t.Context(), "0", request.Identity.RunID, 10)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		row, err := f.lookup(request)
		require.NoError(t, err)
		require.Equal(t, request.Identity.RunID, row.RunID)
		require.Equal(t, "success", *row.ExecutorState)
	}
	newest, err := f.svc.GetActivityDetail(t.Context(), "0", requests[102].Identity.RunID, 10)
	require.NoError(t, err)
	require.Len(t, newest.Output, 1)
	count, err = f.svc.DeleteHistory(t.Context(), "0")
	require.NoError(t, err)
	require.EqualValues(t, 100, count)
	runtimeCount(t, f.svc.db.DB, &Activity{}, 0)
	runtimeCount(t, f.svc.db.DB, &ActivityMessage{}, 0)
	runtimeCount(t, f.svc.db.DB, &RuntimeReceipt{}, 103)
	runtimeCount(t, f.svc.db.DB, &RuntimeKeyBinding{}, 103)
	row, err := f.admit(requests[102])
	require.NoError(t, err)
	require.Equal(t, requests[102].Identity.RunID, row.RunID)
	require.Equal(t, "success", *row.ExecutorState)
	var generation RuntimeStoreGeneration
	require.NoError(t, f.svc.db.First(&generation, "id = 1").Error)
	require.Equal(t, f.generation, generation.Generation)
}
