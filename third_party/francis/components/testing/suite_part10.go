// Source: italypaleale/francis/components/testing/suite.go
// Revision: 23cd46831a543a73eec05ad034afd46245225876
// Retrieved: 2026-10-16
// License: MIT
// Deviations: Pinned provider test support restored for default tests; reserved example addresses; oversized suite split without behavior changes.

package comptesting

import (
	"github.com/italypaleale/francis/components"
	"github.com/italypaleale/francis/internal/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func (s Suite) TestDeleteLeasedAlarm(t *testing.T) {
	t.Run("deletes leased alarm successfully", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch some alarms to create valid leases
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7, SpecHostH8},
		})
		require.NoError(t, err)
		require.NotEmpty(t, res, "should have fetched and leased some alarms")

		// Pick the first leased alarm
		lease := res[0]

		// Verify the alarm exists and is leased
		alarmRes, err := s.p.GetLeasedAlarm(ctx, lease)
		require.NoError(t, err)
		assert.NotEmpty(t, alarmRes.ActorType)

		// Delete the leased alarm
		err = s.p.DeleteLeasedAlarm(ctx, lease)
		require.NoError(t, err)

		// Verify the alarm no longer exists
		_, err = s.p.GetLeasedAlarm(ctx, lease)
		require.ErrorIs(t, err, components.ErrNoAlarm)

		// Also verify using standard GetAlarm that it's completely gone
		alarmRef := ref.AlarmRef{
			ActorType: alarmRes.ActorType,
			ActorID:   alarmRes.ActorID,
			Name:      alarmRes.Name,
		}
		_, err = s.p.GetAlarm(ctx, alarmRef)
		require.ErrorIs(t, err, components.ErrNoAlarm, "alarm should be completely deleted")
	})

	t.Run("returns ErrNoAlarm for non-existent alarm", func(t *testing.T) {
		ctx := t.Context()

		// Seed with empty database
		require.NoError(t, s.p.Seed(ctx, Spec{}))

		// Try to delete a non-existent alarm
		nonExistentLease := ref.NewAlarmLease(ref.NewAlarmRef("at", "aid", "name"), "e7acafab-d2a1-4e95-929f-7da0c781fee0", s.p.Now(), "3c6f3e23-599b-4220-a86a-4a74b3a2ff52")
		err := s.p.DeleteLeasedAlarm(ctx, nonExistentLease)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})

	// An alarm with no lease is one whose actor deactivated, which is what an actor halting itself from its own handler does
	// Finalizing the occurrence that execution just ran has to survive that, or it is left behind to be leased and delivered again
	// It is safe to accept because the lease still names the occurrence it executed: the alarm ID, since replacing an alarm by name mints a new one, and the due time, since a recurrence's next occurrence is always scheduled later
	t.Run("deletes an alarm whose lease its own actor released", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		leases, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH1},
		})
		require.NoError(t, err)
		require.NotEmpty(t, leases)
		lease := leases[0]

		// The actor halting itself from its own handler is what releases the lease
		err = s.p.ReleaseAlarmLease(ctx, lease)
		require.NoError(t, err)

		err = s.p.DeleteLeasedAlarm(ctx, lease)
		require.NoError(t, err)

		_, err = s.p.GetAlarm(ctx, lease.AlarmRef())
		require.ErrorIs(t, err, components.ErrNoAlarm, "the finalized occurrence must be gone")
	})

	// A released lease no longer pins the occurrence, so another host can take it, run it, and reschedule the recurrence under the same alarm ID
	// The stale execution must not finalize what replaced it, which the due time is what says: the occurrence it ran is not the one now scheduled
	t.Run("returns ErrNoAlarm for a released lease on an occurrence that has moved on", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		leases, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH1},
		})
		require.NoError(t, err)
		require.NotEmpty(t, leases)
		lease := leases[0]

		// Another execution moves the occurrence on and leaves the row unleased, which is what rescheduling a recurrence does
		err = s.p.UpdateLeasedAlarm(ctx, lease, components.UpdateLeasedAlarmReq{DueTime: s.p.Now().Add(time.Hour)})
		require.NoError(t, err)

		err = s.p.DeleteLeasedAlarm(ctx, lease)
		require.ErrorIs(t, err, components.ErrNoAlarm)

		_, err = s.p.GetAlarm(ctx, lease.AlarmRef())
		require.NoError(t, err, "the occurrence that replaced it must survive")
	})

	// A lease that merely expired keeps its id, so it names an occurrence this execution no longer owns and must not remove
	t.Run("returns ErrNoAlarm for an expired lease", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		leases, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH1},
		})
		require.NoError(t, err)
		require.NotEmpty(t, leases)
		lease := leases[0]

		// Past the lease duration the alarm is another host's to take, and this execution may no longer finalize it
		_ = s.p.AdvanceClock(2 * time.Minute) //nolint:errcheck

		err = s.p.DeleteLeasedAlarm(ctx, lease)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})

	t.Run("returns ErrNoAlarm for wrong lease ID", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch some alarms to create valid leases
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7, SpecHostH8},
		})
		require.NoError(t, err)
		require.NotEmpty(t, res, "should have fetched and leased some alarms")

		// Pick the first leased alarm
		validLease := res[0]

		// Create a fake lease with wrong lease ID
		fakeLease := ref.NewAlarmLease(validLease.AlarmRef(), validLease.Key(), validLease.DueTime(), "9779ea8f-fa1e-4c2e-9487-bc690b1b57be")

		// Try to delete with wrong lease ID
		err = s.p.DeleteLeasedAlarm(ctx, fakeLease)
		require.ErrorIs(t, err, components.ErrNoAlarm)

		// Verify original lease is still valid
		_, err = s.p.GetLeasedAlarm(ctx, validLease)
		require.NoError(t, err, "original lease should still be valid")
	})

	t.Run("returns ErrNoAlarm for expired lease", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch some alarms to create valid leases
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7, SpecHostH8},
		})
		require.NoError(t, err)
		require.NotEmpty(t, res, "should have fetched and leased some alarms")

		// Pick the first leased alarm
		lease := res[0]

		// Advance time beyond lease expiration (lease duration is 1 minute from GetProviderConfig)
		_ = s.p.AdvanceClock(2 * time.Minute) //nolint:errcheck

		// Try to delete the now-expired lease
		err = s.p.DeleteLeasedAlarm(ctx, lease)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})

	t.Run("idempotent deletion - deleting same alarm twice", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch some alarms to create valid leases
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7, SpecHostH8},
		})
		require.NoError(t, err)
		require.NotEmpty(t, res, "should have fetched and leased some alarms")

		// Pick the first leased alarm
		lease := res[0]

		// Delete the alarm first time - should succeed
		err = s.p.DeleteLeasedAlarm(ctx, lease)
		require.NoError(t, err)

		// Delete the same alarm second time - should return ErrNoAlarm
		err = s.p.DeleteLeasedAlarm(ctx, lease)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})

	t.Run("multiple deletions work independently", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch multiple alarms to create valid leases
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7, SpecHostH8},
		})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(res), 3, "should have fetched at least 3 alarms for this test")

		// Take the first 3 leases
		lease1 := res[0]
		lease2 := res[1]
		lease3 := res[2]

		// Delete lease1 and lease3, but leave lease2
		err = s.p.DeleteLeasedAlarm(ctx, lease1)
		require.NoError(t, err)

		err = s.p.DeleteLeasedAlarm(ctx, lease3)
		require.NoError(t, err)

		// Verify lease1 and lease3 are completely gone
		_, err = s.p.GetLeasedAlarm(ctx, lease1)
		require.ErrorIs(t, err, components.ErrNoAlarm, "lease1 should be deleted")

		_, err = s.p.GetLeasedAlarm(ctx, lease3)
		require.ErrorIs(t, err, components.ErrNoAlarm, "lease3 should be deleted")

		// Verify lease2 is still valid
		_, err = s.p.GetLeasedAlarm(ctx, lease2)
		require.NoError(t, err, "lease2 should still be valid")
	})
}
