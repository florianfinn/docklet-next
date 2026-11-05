// Source: docklet-next/issues/346
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original real coordinator and actor alarm wall-clock proofs.

package runs

import (
	"testing"
	"time"

	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
	kit "go.getarcane.app/kit/pkg"
)

func instant(t *testing.T, value string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)
	return v
}

func TestAlarmUsesRecordZoneAndWallRule(t *testing.T) {
	for _, tc := range []struct{ name, expression, zone, due, want string }{
		{"fold", "0 30 2 * * *", "Europe/Berlin", "2026-10-25T02:30:00+02:00", "2026-10-26T02:30:00+01:00"},
		{"gap", "0 30 2 * * *", "Europe/Berlin", "2027-03-28T03:10:00+02:00", "2027-03-28T03:30:00+02:00"},
		{"earliest", "0 15,30 2 * * *", "Australia/Lord_Howe", "2027-10-03T01:59:00+10:30", "2027-10-03T02:30:00+11:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := newResolutionQueueInternal(t)
			now := instant(t, tc.due)
			q.now = func() time.Time { return now }
			require.NoError(t, q.Checkpoint(t.Context(), tc.name, tc.expression, tc.zone, now))
			before, err := q.ScheduleState(t.Context(), tc.name)
			require.NoError(t, err)
			q.enabled.Store(true)
			defer q.enabled.Store(false)
			actor := &coordinatorActorInternal{id: kit.SHA256Hex("0\x00" + tc.name), q: q}
			require.NoError(t, actor.Alarm(t.Context(), "schedule", occurrenceEnvelopeInternal{st.ScheduleOccurrence{Generation: before.Generation, Sequence: before.Sequence}}))
			after, err := q.ScheduleState(t.Context(), tc.name)
			require.NoError(t, err)
			require.True(t, instant(t, tc.want).Equal(after.NextRun), "next run: %s", after.NextRun)
			require.Len(t, after.Runs, 1)
		})
	}
}

func TestZoneCheckpointFencesOldAlarmWithoutCatchup(t *testing.T) {
	q := newResolutionQueueInternal(t)
	now := instant(t, "2027-01-01T00:00:00Z")
	q.now = func() time.Time { return now }
	require.NoError(t, q.Checkpoint(t.Context(), "zone", "0 0 4 * * *", "UTC", now.Add(time.Hour)))
	old, err := q.ScheduleState(t.Context(), "zone")
	require.NoError(t, err)
	run, err := q.Submit(t.Context(), st.Request{JobID: "zone", Trigger: "manual"})
	require.NoError(t, err)
	require.NoError(t, q.Checkpoint(t.Context(), "zone", old.Schedule, "Asia/Tokyo", now.Add(19*time.Hour)))
	current, err := q.ScheduleState(t.Context(), "zone")
	require.NoError(t, err)
	require.Greater(t, current.Generation, old.Generation)
	require.Equal(t, "Asia/Tokyo", current.Zone)
	require.Len(t, current.Runs, 1)
	require.Equal(t, run.ID, current.Runs[0].ID)
	q.enabled.Store(true)
	defer q.enabled.Store(false)
	actor := &coordinatorActorInternal{id: kit.SHA256Hex("0\x00zone"), q: q}
	require.NoError(t, actor.Alarm(t.Context(), "old", occurrenceEnvelopeInternal{st.ScheduleOccurrence{Generation: old.Generation, Sequence: old.Sequence}}))
	after, err := q.ScheduleState(t.Context(), "zone")
	require.NoError(t, err)
	require.True(t, after.NextRun.Equal(current.NextRun))
	require.Len(t, after.Runs, 1)
	// An upgraded checkpoint without a zone is recomputed once, even at startup.
	require.NoError(t, q.mutateInternal(t.Context(), "0", "legacy", func(record *st.QueueRecord) error {
		record.Schedule, record.NextRun = "0 0 4 * * *", now.Add(-time.Hour)
		return nil
	}))
	require.NoError(t, q.Checkpoint(t.Context(), "legacy", "0 0 4 * * *", "UTC", now.Add(4*time.Hour)))
	legacy, err := q.ScheduleState(t.Context(), "legacy")
	require.NoError(t, err)
	require.Empty(t, legacy.Runs)
	require.Equal(t, "UTC", legacy.Zone)
}
