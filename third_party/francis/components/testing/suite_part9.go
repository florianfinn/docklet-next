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

func (s Suite) TestRenewAlarmLeases(t *testing.T) {
	t.Run("renews leases for specific hosts", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch some alarms to create valid leases
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7, SpecHostH8},
		})
		require.NoError(t, err)
		require.NotEmpty(t, res, "should have fetched and leased some alarms")

		// Advance time partway through lease duration to simulate renewal scenario
		_ = s.p.AdvanceClock(30 * time.Second) //nolint:errcheck

		// Renew leases for H7 only
		renewReq := components.RenewAlarmLeasesReq{
			Hosts: []string{SpecHostH7},
		}
		renewRes, err := s.p.RenewAlarmLeases(ctx, renewReq)
		require.NoError(t, err)
		require.NotEmpty(t, renewRes.Leases, "should have renewed some leases")

		// Verify all returned leases are still valid
		for _, lease := range renewRes.Leases {
			_, err := s.p.GetLeasedAlarm(ctx, lease)
			require.NoError(t, err, "renewed lease should be valid")
		}

		// Advance time beyond original lease expiration
		// Total: 75 seconds (beyond original 60s lease)
		_ = s.p.AdvanceClock(45 * time.Second) //nolint:errcheck

		// Renewed leases should still be valid (they were extended)
		for _, lease := range renewRes.Leases {
			_, err := s.p.GetLeasedAlarm(ctx, lease)
			require.NoError(t, err, "renewed lease should still be valid after original expiration")
		}
	})

	t.Run("renews specific leases only", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch some alarms to create valid leases
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7, SpecHostH8},
		})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(res), 3, "need at least 3 leases for this test")

		// Select first 2 leases for renewal
		leasesToRenew := []*ref.AlarmLease{res[0], res[1]}
		leaseNotRenewed := res[2]

		// Advance time partway through lease duration
		_ = s.p.AdvanceClock(30 * time.Second) //nolint:errcheck

		// Renew only specific leases
		renewReq := components.RenewAlarmLeasesReq{
			Hosts:  []string{SpecHostH7, SpecHostH8},
			Leases: leasesToRenew,
		}
		renewRes, err := s.p.RenewAlarmLeases(ctx, renewReq)
		require.NoError(t, err)
		require.Len(t, renewRes.Leases, 2, "should have renewed exactly 2 leases")

		// Verify the specific leases were renewed
		renewedLeaseKeys := make(map[string]bool)
		for _, lease := range renewRes.Leases {
			renewedLeaseKeys[lease.Key()] = true
		}
		assert.True(t, renewedLeaseKeys[leasesToRenew[0].Key()], "first lease should be renewed")
		assert.True(t, renewedLeaseKeys[leasesToRenew[1].Key()], "second lease should be renewed")

		// Advance time beyond original lease expiration
		// Total: 75 seconds
		_ = s.p.AdvanceClock(45 * time.Second) //nolint:errcheck

		// Renewed leases should still be valid
		for _, lease := range renewRes.Leases {
			_, err := s.p.GetLeasedAlarm(ctx, lease)
			require.NoError(t, err, "renewed lease should still be valid")
		}

		// Non-renewed lease should have expired
		_, err = s.p.GetLeasedAlarm(ctx, leaseNotRenewed)
		require.ErrorIs(t, err, components.ErrNoAlarm, "non-renewed lease should have expired")
	})

	t.Run("returns empty result when no matching leases", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Try to renew leases for hosts with no active leases
		renewReq := components.RenewAlarmLeasesReq{
			// These hosts don't have any leases in the initial seed
			Hosts: []string{SpecHostH1, SpecHostH2},
		}
		renewRes, err := s.p.RenewAlarmLeases(ctx, renewReq)
		require.NoError(t, err)
		assert.Empty(t, renewRes.Leases, "should return empty result when no matching leases")
	})

	t.Run("returns empty result for non-existent hosts", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Try to renew leases for non-existent hosts
		renewReq := components.RenewAlarmLeasesReq{
			Hosts: []string{"dd86ac68-ee00-4a8f-97e1-a3d4bbb92e0f", "e8a3256c-5381-4a3c-b8f7-19dc87913d5f"},
		}
		renewRes, err := s.p.RenewAlarmLeases(ctx, renewReq)
		require.NoError(t, err)
		assert.Empty(t, renewRes.Leases, "should return empty result for non-existent hosts")
	})

	t.Run("ignores expired leases", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch some alarms to create valid leases
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7, SpecHostH8},
		})
		require.NoError(t, err)
		require.NotEmpty(t, res, "should have fetched and leased some alarms")

		// Advance time beyond lease expiration (1 minute)
		_ = s.p.AdvanceClock(2 * time.Minute) //nolint:errcheck

		// Try to renew expired leases
		renewReq := components.RenewAlarmLeasesReq{
			Hosts: []string{SpecHostH7, SpecHostH8},
		}
		renewRes, err := s.p.RenewAlarmLeases(ctx, renewReq)
		require.NoError(t, err)
		assert.Empty(t, renewRes.Leases, "should not renew expired leases")
	})

	t.Run("handles mixed valid and invalid lease IDs", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch some alarms to create valid leases
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7, SpecHostH8},
		})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(res), 2, "need at least 2 leases for this test")

		// Create a mix of valid and invalid lease IDs
		validLease := res[0]
		invalidLease := ref.NewAlarmLease(validLease.AlarmRef(), "46d1668d-dd68-4320-a562-66176ac4a11f", s.p.Now(), "8a54df5a-2007-4add-af3c-265c4d569e28")

		// Advance time partway through lease duration
		_ = s.p.AdvanceClock(30 * time.Second) //nolint:errcheck

		// Try to renew mix of valid and invalid leases
		renewReq := components.RenewAlarmLeasesReq{
			Hosts:  []string{SpecHostH7, SpecHostH8},
			Leases: []*ref.AlarmLease{validLease, invalidLease},
		}
		renewRes, err := s.p.RenewAlarmLeases(ctx, renewReq)
		require.NoError(t, err)

		// Should only renew the valid lease
		require.Len(t, renewRes.Leases, 1, "should renew only the valid lease")
		assert.Equal(t, validLease.Key(), renewRes.Leases[0].Key(), "should renew the correct lease")
	})

	t.Run("renews all leases for multiple hosts", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch alarms from H7
		res1, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7},
		})
		require.NoError(t, err)
		require.NotEmpty(t, res1)

		// Fetch alarms from H8
		res2, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH8},
		})
		require.NoError(t, err)
		require.NotEmpty(t, res2)

		totalExpectedLeases := len(res1) + len(res2)

		// Advance time partway through lease duration
		_ = s.p.AdvanceClock(30 * time.Second) //nolint:errcheck

		// Renew all leases for both hosts
		renewReq := components.RenewAlarmLeasesReq{
			Hosts: []string{SpecHostH7, SpecHostH8},
		}
		renewRes, err := s.p.RenewAlarmLeases(ctx, renewReq)
		require.NoError(t, err)
		assert.Len(t, renewRes.Leases, totalExpectedLeases, "should renew all leases from both hosts")

		// Verify all renewed leases are valid
		for _, lease := range renewRes.Leases {
			_, err := s.p.GetLeasedAlarm(ctx, lease)
			require.NoError(t, err, "all renewed leases should be valid")
		}
	})

	t.Run("empty host list renews nothing", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Create some valid leases
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7, SpecHostH8},
		})
		require.NoError(t, err)
		require.NotEmpty(t, res, "should have fetched and leased some alarms")

		// Renewal with no hosts must not error and must renew nothing
		renewRes, err := s.p.RenewAlarmLeases(ctx, components.RenewAlarmLeasesReq{})
		require.NoError(t, err)
		assert.Empty(t, renewRes.Leases, "no leases should be renewed without a host list")

		// Even when specific leases are provided, an empty host list renews nothing
		renewRes, err = s.p.RenewAlarmLeases(ctx, components.RenewAlarmLeasesReq{
			Leases: []*ref.AlarmLease{res[0]},
		})
		require.NoError(t, err)
		assert.Empty(t, renewRes.Leases, "no leases should be renewed without a host list, even when leases are specified")
	})
}

func (s Suite) TestReleaseAlarmLease(t *testing.T) {
	t.Run("releases valid lease successfully", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch some alarms to create valid leases
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7, SpecHostH8},
		})
		require.NoError(t, err)
		require.NotEmpty(t, res, "should have fetched and leased some alarms")

		// Pick the first leased alarm to test with
		lease := res[0]

		// Verify the alarm is leased
		alarmRes, err := s.p.GetLeasedAlarm(ctx, lease)
		require.NoError(t, err)
		assert.NotEmpty(t, alarmRes.ActorType)

		// Release the lease
		err = s.p.ReleaseAlarmLease(ctx, lease)
		require.NoError(t, err)

		// Verify the lease is no longer valid
		_, err = s.p.GetLeasedAlarm(ctx, lease)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})

	t.Run("returns ErrNoAlarm for non-existent alarm", func(t *testing.T) {
		ctx := t.Context()

		// Seed with empty database
		require.NoError(t, s.p.Seed(ctx, Spec{}))

		// Try to release a non-existent alarm lease
		nonExistentLease := ref.NewAlarmLease(ref.NewAlarmRef("at", "aid", "name"), "7f84f417-de01-46d0-b5bb-80f3d8bf003b", s.p.Now(), "46d03825-2a8f-498d-bd02-e1e6bf8d82c3")
		err := s.p.ReleaseAlarmLease(ctx, nonExistentLease)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})

	t.Run("returns ErrNoAlarm for alarm with no lease", func(t *testing.T) {
		ctx := t.Context()

		// Create a custom test spec with an unleased alarm
		customSpec := Spec{
			Hosts: []HostSpec{
				{HostID: SpecHostH1, Address: "127.0.0.1:4001", LastHealthAgo: 2 * time.Second},
			},
			HostActorTypes: []HostActorTypeSpec{
				{HostID: SpecHostH1, ActorType: "TestType", ActorIdleTimeout: 5 * time.Minute, ActorConcurrencyLimit: 0},
			},
			Alarms: []AlarmSpec{
				{
					AlarmID:   "0ec13fd1-ff6d-4059-bcc6-29315f57b1c6",
					ActorType: "TestType",
					ActorID:   "test-actor",
					Name:      "test-alarm",
					DueIn:     time.Second,
					Data:      []byte("test-data"),
				},
			},
		}

		// Seed with custom data
		require.NoError(t, s.p.Seed(ctx, customSpec))

		// Try to release a lease for an alarm that was never leased
		fakeLease := ref.NewAlarmLease(ref.NewAlarmRef("TestType", "test-actor", "test-alarm"), "0ec13fd1-ff6d-4059-bcc6-29315f57b1c6", s.p.Now(), "e811769a-6b7a-4080-b406-87efd603c7f4")
		err := s.p.ReleaseAlarmLease(ctx, fakeLease)
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
		lease := res[0]

		// Create a fake lease with wrong lease ID
		fakeLease := ref.NewAlarmLease(lease.AlarmRef(), lease.Key(), lease.DueTime(), "e731a719-0c1c-4c41-9c92-a44e0e8ef681")

		// Try to release with wrong lease ID
		err = s.p.ReleaseAlarmLease(ctx, fakeLease)
		require.ErrorIs(t, err, components.ErrNoAlarm)

		// Verify original lease is still valid
		_, err = s.p.GetLeasedAlarm(ctx, lease)
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

		// Try to release the now-expired lease
		err = s.p.ReleaseAlarmLease(ctx, lease)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})

	t.Run("idempotent release - releasing same lease twice", func(t *testing.T) {
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

		// Release the lease first time - should succeed
		err = s.p.ReleaseAlarmLease(ctx, lease)
		require.NoError(t, err)

		// Release the same lease second time - should return ErrNoAlarm
		err = s.p.ReleaseAlarmLease(ctx, lease)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})

	t.Run("multiple releases work independently", func(t *testing.T) {
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

		// Release lease1 and lease3, but leave lease2
		err = s.p.ReleaseAlarmLease(ctx, lease1)
		require.NoError(t, err)

		err = s.p.ReleaseAlarmLease(ctx, lease3)
		require.NoError(t, err)

		// Verify lease1 and lease3 are no longer valid
		_, err = s.p.GetLeasedAlarm(ctx, lease1)
		require.ErrorIs(t, err, components.ErrNoAlarm, "lease1 should be released")

		_, err = s.p.GetLeasedAlarm(ctx, lease3)
		require.ErrorIs(t, err, components.ErrNoAlarm, "lease3 should be released")

		// Verify lease2 is still valid
		_, err = s.p.GetLeasedAlarm(ctx, lease2)
		require.NoError(t, err, "lease2 should still be valid")
	})
}

func (s Suite) TestUpdateLeasedAlarm(t *testing.T) {
	t.Run("updates alarm with refresh lease", func(t *testing.T) {
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

		// Get original alarm details
		originalAlarm, err := s.p.GetLeasedAlarm(ctx, lease)
		require.NoError(t, err)

		// Update the alarm with new details and refresh lease
		newDueTime := s.p.Now().Add(2 * time.Hour)
		updateReq := components.UpdateLeasedAlarmReq{
			DueTime:      newDueTime,
			RefreshLease: true,
		}

		err = s.p.UpdateLeasedAlarm(ctx, lease, updateReq)
		require.NoError(t, err)

		// Verify the alarm was updated and lease is still valid
		updatedAlarm, err := s.p.GetLeasedAlarm(ctx, lease)
		require.NoError(t, err)

		// Check updated fields
		assert.Equal(t, newDueTime.UnixMilli(), updatedAlarm.DueTime.UnixMilli())

		// Verify other fields remain unchanged
		assert.Equal(t, originalAlarm.ActorType, updatedAlarm.ActorType)
		assert.Equal(t, originalAlarm.ActorID, updatedAlarm.ActorID)
		assert.Equal(t, originalAlarm.Name, updatedAlarm.Name)
		assert.Equal(t, originalAlarm.Data, updatedAlarm.Data)
		assert.Equal(t, originalAlarm.Interval, updatedAlarm.Interval)
		if originalAlarm.TTL == nil {
			require.Nil(t, updatedAlarm.TTL)
		} else {
			require.NotNil(t, updatedAlarm.TTL)
			assert.Equal(t, originalAlarm.TTL.UnixMilli(), updatedAlarm.TTL.UnixMilli())
		}
	})

	t.Run("updates alarm without refresh lease", func(t *testing.T) {
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

		// Update the alarm without refreshing lease
		// This will release the lease
		newDueTime := s.p.Now().Add(3 * time.Hour)
		updateReq := components.UpdateLeasedAlarmReq{
			DueTime:      newDueTime,
			RefreshLease: false,
		}

		err = s.p.UpdateLeasedAlarm(ctx, lease, updateReq)
		require.NoError(t, err)

		// Verify the lease is no longer valid (was released)
		_, err = s.p.GetLeasedAlarm(ctx, lease)
		require.ErrorIs(t, err, components.ErrNoAlarm, "lease should be released")
	})

	t.Run("returns ErrNoAlarm for non-existent alarm", func(t *testing.T) {
		ctx := t.Context()

		// Seed with empty database
		require.NoError(t, s.p.Seed(ctx, Spec{}))

		// Try to update a non-existent alarm
		nonExistentLease := ref.NewAlarmLease(ref.NewAlarmRef("at", "aid", "name"), "0b610c71-fd4a-429b-a4ff-873698e5b3a1", s.p.Now(), "8693c0bc-1062-405e-9e7b-3a9c2f65899d")
		updateReq := components.UpdateLeasedAlarmReq{
			DueTime:      s.p.Now().Add(1 * time.Hour),
			RefreshLease: true,
		}

		err := s.p.UpdateLeasedAlarm(ctx, nonExistentLease, updateReq)
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

		// Pick the first leased alarm and create fake lease with wrong ID
		validLease := res[0]
		fakeLease := ref.NewAlarmLease(validLease.AlarmRef(), validLease.Key(), validLease.DueTime(), "ebccb3b6-b677-437b-83d1-cb0b21134328")

		updateReq := components.UpdateLeasedAlarmReq{
			DueTime:      s.p.Now().Add(1 * time.Hour),
			RefreshLease: true,
		}

		err = s.p.UpdateLeasedAlarm(ctx, fakeLease, updateReq)
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

		updateReq := components.UpdateLeasedAlarmReq{
			DueTime:      s.p.Now().Add(1 * time.Hour),
			RefreshLease: true,
		}

		// Try to update the now-expired lease
		err = s.p.UpdateLeasedAlarm(ctx, lease, updateReq)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})
}
