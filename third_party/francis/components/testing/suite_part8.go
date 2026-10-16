// Source: italypaleale/francis/components/testing/suite.go
// Revision: 23cd46831a543a73eec05ad034afd46245225876
// Retrieved: 2026-10-16
// License: MIT
// Deviations: Pinned provider test support restored for default tests; reserved example addresses; oversized suite split without behavior changes.

package comptesting

import (
	"fmt"
	"github.com/italypaleale/francis/components"
	"github.com/italypaleale/francis/internal/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"slices"
	"strings"
	"testing"
	"time"
)

func (s Suite) TestFetchAlarms(t *testing.T) {
	// In the seed data, ALM-C-001...ALM-C-005 are already leased with a valid lease
	// ALM-C-006 has an expired lease
	expectPreLeasedAlarms := func(alarmID string) bool {
		switch alarmID {
		case "AA000000-000C-4000-000C-000000000001",
			"AA000000-000C-4000-000C-000000000002",
			"AA000000-000C-4000-000C-000000000003",
			"AA000000-000C-4000-000C-000000000004",
			"AA000000-000C-4000-000C-000000000005",
			"AA000000-000C-4000-000C-000000000006":
			return true
		default:
			return false
		}
	}

	t.Run("fetches upcoming alarms without capacity constraints", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Retrieve the alarms
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7},
		})
		require.NoError(t, err)

		// This should return a total of 24 alarms, all of types X and Y
		// Alarms ALM-X-002 and ALM-Y-001 (for actors X-2 and Y-1) should not be returned because the actors are active on H8
		// (and that's why we iterate till 13)
		// Alarm ALM-Y-002 should be included even though it's active on actor Y-2, because it's on H9 which is unhealthy
		expectAlarmIDs := make([]string, 0, 24)
		expectAlarmIDsMap := make(map[string]bool, 24)
		expectActorIDs := make([]string, 0, 24)
		for _, typ := range []string{"X", "Y"} {
			for i := 1; i <= 13; i++ {
				if (typ == "X" && i == 2) || (typ == "Y" && i == 1) {
					continue
				}

				var alarmID string
				if typ == "X" {
					alarmID = fmt.Sprintf("AA000000-EEEE-4000-00EE-000000000%03d", i)
				} else {
					alarmID = fmt.Sprintf("AA000000-FFFF-4000-00FF-000000000%03d", i)
				}
				actorID := fmt.Sprintf("%s-%d", typ, i)

				expectAlarmIDs = append(expectAlarmIDs, alarmID)
				expectAlarmIDsMap[alarmID] = true
				expectActorIDs = append(expectActorIDs, actorID)
			}
		}

		// Collect all alarm IDs
		gotIDs := make([]string, 0, 24)
		for _, a := range res {
			gotIDs = append(gotIDs, strings.ToUpper(a.Key()))
			assert.NotEmpty(t, a.LeaseID())
		}

		// Order doesn't matter
		slices.Sort(expectAlarmIDs)
		slices.Sort(gotIDs)
		assert.Equal(t, expectAlarmIDs, gotIDs)

		// Ensure that the alarms' leases were acquired in the database, and only for the alarms we retrieved
		spec, err := s.p.GetAllHosts(t.Context())
		require.NoError(t, err)

		for _, a := range spec.Alarms {
			alarmID := strings.ToUpper(a.AlarmID)

			if expectPreLeasedAlarms(alarmID) {
				continue
			}

			if !expectAlarmIDsMap[alarmID] {
				assert.Emptyf(t, a.LeaseID, "expected alarm %q not to have a lease ID", alarmID)
				assert.Emptyf(t, a.LeaseExp, "expected alarm %q not to have a lease expiration", alarmID)
				continue
			}

			_ = assert.NotNil(t, a.LeaseID, "expected alarm %q to have a lease ID", alarmID) &&
				assert.NotEmpty(t, *a.LeaseID, "expected alarm %q to have a lease ID", alarmID)
			_ = assert.NotNil(t, a.LeaseExp, "expected alarm %q to have a lease expiration", alarmID) &&
				assert.Greater(t, *a.LeaseExp, s.p.Now(), "expected alarm's %q lease expiration to be in the future", alarmID)
		}

		// Also ensure that all actors were activated on H7
		// Note that seed data contains active actors already
		gotActiveActorIDs := make(map[string]string, len(spec.ActiveActors))
		for _, a := range spec.ActiveActors {
			gotActiveActorIDs[a.ActorID] = a.HostID
		}

		for _, id := range expectActorIDs {
			_ = assert.NotEmptyf(t, gotActiveActorIDs[id], "expected actor %q to be active on host H7, but it was not active", id) &&
				assert.Equalf(t, SpecHostH7, gotActiveActorIDs[id], "expected actor %q to be active on host H7, but it was active on host %q", id, gotActiveActorIDs[id])
		}
	})

	t.Run("fetches upcoming alarms with capacity constraints", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Retrieve the alarms
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH1, SpecHostH2},
		})
		require.NoError(t, err)

		// This should return a total of 24 alarms, all of types A, B, and C
		// Type A doesn't have any capacity left, but actors A-1, A-2, A-4 are active on H1 and H2, so alarms ALM-A-1, ALM-A-2, ALM-A-4 should be included
		// For type B, the combined capacity between H1 and H2 is 10, with 2 actors already active, so we should only get the earliest 8 plus ALM-B-1 and ALM-B-2 which are for the actors active on H1 and H2 (meanwhile, ALM-B-3 is active on H3 so should not be returned)
		// There's no capacity limit on type C, so we should get 12 of them. However, ALM-C-001...ALM-C-005 are already leased with a valid lease
		expectAlarmIDs := []string{
			SpecAlarmA1, SpecAlarmA2, SpecAlarmA4,
			SpecAlarmB1, SpecAlarmB2,
			"AA000000-000B-4000-000B-000000000001", // ALM-B-001
			"AA000000-000B-4000-000B-000000000007", // ALM-B-007
			"AA000000-000B-4000-000B-000000000014", // ALM-B-014
			"AA000000-000B-4000-000B-000000000021", // ALM-B-021
			"AA000000-000B-4000-000B-000000000028", // ALM-B-028
			"AA000000-000B-4000-000B-000000000035", // ALM-B-035
			"AA000000-000B-4000-000B-000000000042", // ALM-B-042
			"AA000000-000B-4000-000B-000000000049", // ALM-B-049
			"AA000000-000C-4000-000C-000000000006", // ALM-C-006
			"AA000000-000C-4000-000C-000000000010", // ALM-C-010
			"AA000000-000C-4000-000C-000000000011", // ALM-C-011
			"AA000000-000C-4000-000C-000000000015", // ALM-C-015
			"AA000000-000C-4000-000C-000000000020", // ALM-C-020
			"AA000000-000C-4000-000C-000000000025", // ALM-C-025
			"AA000000-000C-4000-000C-000000000030", // ALM-C-030
			"AA000000-000C-4000-000C-000000000035", // ALM-C-035
			"AA000000-000C-4000-000C-000000000040", // ALM-C-040
			"AA000000-000C-4000-000C-000000000045", // ALM-C-045
			"AA000000-000C-4000-000C-000000000050", // ALM-C-050
		}
		expectActorIDs := []string{
			"A-1", "A-2", "A-4", "B-1", "B-2",
			"B-001", "B-007", "B-014", "B-021", "B-028", "B-035", "B-042", "B-049",
			"C-006", "C-010", "C-011", "C-015", "C-020", "C-025", "C-030", "C-035", "C-040", "C-045", "C-050",
		}
		expectAlarmIDsMap := make(map[string]bool, len(expectAlarmIDs))
		for _, id := range expectAlarmIDs {
			expectAlarmIDsMap[id] = true
		}

		// Collect all alarm IDs
		gotIDs := make([]string, 0, 24)
		for _, a := range res {
			gotIDs = append(gotIDs, strings.ToUpper(a.Key()))
			assert.NotEmpty(t, a.LeaseID())
		}

		// Order doesn't matter
		slices.Sort(expectAlarmIDs)
		slices.Sort(gotIDs)
		assert.Equal(t, expectAlarmIDs, gotIDs)

		// Ensure that the alarms' leases were acquired in the database, and only for the alarms we retrieved
		spec, err := s.p.GetAllHosts(t.Context())
		require.NoError(t, err)

		for _, a := range spec.Alarms {
			alarmID := strings.ToUpper(a.AlarmID)

			// ALM-C-006's leases was expired and we should have taken it over
			if a.AlarmID != "AA000000-000C-4000-000C-000000000006" && expectPreLeasedAlarms(alarmID) {
				continue
			}

			if !expectAlarmIDsMap[alarmID] {
				// Seed data doesn't contain any leased alarm, so we can confidently exclude others
				assert.Emptyf(t, a.LeaseID, "expected alarm %q not to have a lease ID", alarmID)
				assert.Emptyf(t, a.LeaseExp, "expected alarm %q not to have a lease expiration", alarmID)
				continue
			}

			_ = assert.NotNil(t, a.LeaseID, "expected alarm %q to have a lease ID", alarmID) &&
				assert.NotEmpty(t, *a.LeaseID, "expected alarm %q to have a lease ID", alarmID)
			_ = assert.NotNil(t, a.LeaseExp, "expected alarm %q to have a lease expiration", alarmID) &&
				assert.Greater(t, *a.LeaseExp, s.p.Now(), "expected alarm's %q lease expiration to be in the future", alarmID)
		}

		// Also ensure that all actors were activated on H1 or H2
		// Note that seed data contains active actors already
		gotActiveActorIDs := make(map[string]string, len(spec.ActiveActors))
		for _, a := range spec.ActiveActors {
			gotActiveActorIDs[a.ActorID] = a.HostID
		}

		hostCounts := make(map[string]int, 2)
		for _, id := range expectActorIDs {
			if !assert.NotEmptyf(t, gotActiveActorIDs[id], "expected actor %q to be active on a host, but it was not active", id) {
				continue
			}

			switch id {
			// These actors were already active in the seed data
			case "A-1", "A-2", "B-1":
				assert.Equalf(t, SpecHostH1, gotActiveActorIDs[id], "expected actor %q to be active on host H1, but it was active on host %q", id, gotActiveActorIDs[id])
			case "A-4", "B-2":
				assert.Equalf(t, SpecHostH2, gotActiveActorIDs[id], "expected actor %q to be active on host H2, but it was active on host %q", id, gotActiveActorIDs[id])
			default:
				assert.Contains(t, []string{SpecHostH1, SpecHostH2}, gotActiveActorIDs[id], "expected actor %q to be active on host H1 or H2, but it was active on host %q", id, gotActiveActorIDs[id])
				hostCounts[gotActiveActorIDs[id]]++
			}
		}

		// There should be some level of distribution for actors that were just activated
		// It doesn't have to be 50/50 since there's randomness involved
		assert.Len(t, hostCounts, 2)
		assert.GreaterOrEqual(t, hostCounts[SpecHostH1], 4)
		assert.GreaterOrEqual(t, hostCounts[SpecHostH2], 4)
	})

	t.Run("returns empty slice when no hosts provided", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch alarms with empty hosts list
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{},
		})
		require.NoError(t, err)
		assert.Empty(t, res)
	})

	t.Run("returns empty slice when all hosts are unhealthy", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch alarms only from unhealthy hosts
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			// Both unhealthy in seed data
			Hosts: []string{SpecHostH5, SpecHostH6},
		})
		require.NoError(t, err)
		assert.Empty(t, res)
	})

	t.Run("returns empty slice when hosts don't exist", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch alarms from non-existent hosts
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{"95302b0c-92cd-4201-b15d-98fcd56d7bf5", "ee6e3ddd-9ff0-42e7-af80-5624407f6da9"},
		})
		require.NoError(t, err)
		assert.Empty(t, res)
	})

	t.Run("returns empty slice when no upcoming alarms", func(t *testing.T) {
		ctx := t.Context()

		// Seed with hosts but no alarms
		customSpec := Spec{
			Hosts: []HostSpec{
				{HostID: SpecHostH1, Address: "127.0.0.1:4001", LastHealthAgo: 2 * time.Second},
			},
			HostActorTypes: []HostActorTypeSpec{
				{HostID: SpecHostH1, ActorType: "TestType", ActorIdleTimeout: 5 * time.Minute, ActorConcurrencyLimit: 0},
			},
			Alarms: []AlarmSpec{},
		}
		require.NoError(t, s.p.Seed(ctx, customSpec))

		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH1},
		})
		require.NoError(t, err)
		assert.Empty(t, res, "should return empty slice when no upcoming alarms")
	})

	t.Run("doesn't return already leased alarms with valid leases", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// First fetch should get some alarms and lease them
		res1, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7},
		})
		require.NoError(t, err)
		require.NotEmpty(t, res1)

		// Second fetch immediately should not return the same alarms (they're already leased)
		res2, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7},
		})
		require.NoError(t, err)

		// Verify no overlap between the two batches
		leased1 := make(map[string]bool)
		for _, lease := range res1 {
			leased1[lease.Key()] = true
		}

		for _, lease := range res2 {
			assert.False(t, leased1[lease.Key()], "alarm %s should not appear in both batches", lease.Key())
		}
	})

	t.Run("takes over expired leases", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data (contains ALM-C-006 with expired lease)
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch alarms from H1 and H2 where C type is supported
		// This should include ALM-C-006 which has an expired lease
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH1, SpecHostH2},
		})
		require.NoError(t, err)
		require.NotEmpty(t, res)

		// Check if we got the expired lease ALM-C-006 and gave it a new lease
		foundExpiredAlarm := false
		for _, lease := range res {
			if strings.ToUpper(lease.Key()) == "AA000000-000C-4000-000C-000000000006" {
				// Should have a lease ID (new lease was created)
				assert.NotEmpty(t, lease.LeaseID(), "ALM-C-006 should have been given a new lease")
				foundExpiredAlarm = true
				break
			}
		}
		assert.True(t, foundExpiredAlarm, "should have found and taken over the expired lease ALM-C-006")
	})

	t.Run("fetches overdue alarms", func(t *testing.T) {
		ctx := t.Context()

		// Create a custom test spec with overdue alarms
		customSpec := Spec{
			Hosts: []HostSpec{
				{HostID: SpecHostH1, Address: "127.0.0.1:4001", LastHealthAgo: 2 * time.Second}, // healthy
			},
			HostActorTypes: []HostActorTypeSpec{
				{HostID: SpecHostH1, ActorType: "TestOverdue", ActorIdleTimeout: 5 * time.Minute, ActorConcurrencyLimit: 0},
			},
			Alarms: []AlarmSpec{
				{
					AlarmID:   SpecAlarmOverdue1,
					ActorType: "TestOverdue",
					ActorID:   "overdue-actor-1",
					Name:      "overdue-alarm-1",
					DueIn:     -5 * time.Minute, // Due 5 minutes ago (overdue)
					Data:      []byte("overdue-data-1"),
				},
				{
					AlarmID:   SpecAlarmOverdue2,
					ActorType: "TestOverdue",
					ActorID:   "overdue-actor-2",
					Name:      "overdue-alarm-2",
					DueIn:     -30 * time.Second, // Due 30 seconds ago (overdue)
					Data:      []byte("overdue-data-2"),
				},
			},
		}

		// Seed with overdue alarms
		require.NoError(t, s.p.Seed(ctx, customSpec))

		// Fetch alarms - should include overdue ones
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH1},
		})
		require.NoError(t, err)
		require.NotEmpty(t, res, "should fetch overdue alarms")

		// Verify both overdue alarms were fetched
		foundOverdue1 := false
		foundOverdue2 := false
		for _, lease := range res {
			key := strings.ToUpper(lease.Key())
			if key == SpecAlarmOverdue1 {
				foundOverdue1 = true
				// Verify the alarm is in the past
				assert.True(t, lease.DueTime().Before(s.p.Now()), "ALM-OVERDUE-1 should be overdue")
				assert.Equal(t, "TestOverdue/overdue-actor-1", lease.ActorRef().String())
			}
			if key == SpecAlarmOverdue2 {
				foundOverdue2 = true
				// Verify the alarm is in the past
				assert.True(t, lease.DueTime().Before(s.p.Now()), "ALM-OVERDUE-2 should be overdue")
				assert.Equal(t, "TestOverdue/overdue-actor-2", lease.ActorRef().String())
			}
		}
		assert.True(t, foundOverdue1, "should have found overdue alarm ALM-OVERDUE-1")
		assert.True(t, foundOverdue2, "should have found overdue alarm ALM-OVERDUE-2")

		// Verify the leased overdue alarms can be retrieved
		for _, lease := range res {
			key := strings.ToUpper(lease.Key())
			if key == SpecAlarmOverdue1 || key == SpecAlarmOverdue2 {
				alarmRes, err := s.p.GetLeasedAlarm(ctx, lease)
				require.NoError(t, err, "overdue alarm %s should be properly leased", key)
				assert.Equal(t, "TestOverdue", alarmRes.ActorType)
				assert.Equal(t, "TestOverdue/overdue-actor-"+key[len(key)-1:], lease.ActorRef().String())
			}
		}
	})

	t.Run("mixed healthy and unhealthy hosts filters correctly", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Request from mix of healthy and unhealthy hosts
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7, SpecHostH5, SpecHostH8, SpecHostH6}, // H7,H8 healthy, H5,H6 unhealthy
		})
		require.NoError(t, err)
		require.NotEmpty(t, res, "should return alarms from healthy hosts")

		// Verify all returned alarms can be retrieved (meaning they were properly leased)
		for _, lease := range res {
			alarmRes, err := s.p.GetLeasedAlarm(ctx, lease)
			require.NoError(t, err, "alarm %s should be properly leased", lease.Key())
			assert.Contains(t, []string{"X", "Y"}, alarmRes.ActorType, "should only have X/Y type alarms from H7/H8")
		}

		// Verify actors were only placed on healthy hosts
		spec, err := s.p.GetAllHosts(ctx)
		require.NoError(t, err)

		gotActiveActorIDs := make(map[string]string)
		for _, a := range spec.ActiveActors {
			gotActiveActorIDs[a.ActorID] = a.HostID
		}

		for _, lease := range res {
			alarmRes, _ := s.p.GetLeasedAlarm(ctx, lease)
			if hostID, exists := gotActiveActorIDs[alarmRes.ActorID]; exists {
				assert.Contains(t, []string{SpecHostH7, SpecHostH8}, hostID, "actor %s should only be placed on healthy hosts", alarmRes.ActorID)
			}
		}
	})
}

func (s Suite) TestGetLeasedAlarm(t *testing.T) {
	t.Run("returns alarm with valid lease", func(t *testing.T) {
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

		// Get the leased alarm details
		alarmRes, err := s.p.GetLeasedAlarm(ctx, lease)
		require.NoError(t, err)

		// Verify the alarm details
		assert.NotEmpty(t, alarmRes.ActorType)
		assert.NotEmpty(t, alarmRes.ActorID)
		assert.NotEmpty(t, alarmRes.Name)
		assert.Equal(t, lease.DueTime(), alarmRes.DueTime)

		// The alarm should be of type X or Y based on our test data
		assert.Contains(t, []string{"X", "Y"}, alarmRes.ActorType)
	})

	t.Run("returns ErrNoAlarm if alarm doesn't exist", func(t *testing.T) {
		ctx := t.Context()

		// Seed with empty database
		require.NoError(t, s.p.Seed(ctx, Spec{}))

		// Try to get a non-existent alarm
		nonExistentLease := ref.NewAlarmLease(ref.NewAlarmRef("at", "aid", "name"), "cc11a1b4-8c70-4253-8e24-64eb6e876eb6", s.p.Now(), "1e4ecca7-db68-431c-a6d7-08aa0434e5c6")
		_, err := s.p.GetLeasedAlarm(ctx, nonExistentLease)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})

	t.Run("returns ErrNoAlarm if alarm isn't leased", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data (has un-leased alarms)
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Try to get an alarm that exists but isn't leased
		// From spec, ALM-B-007 and later B alarms should not be pre-leased
		unleaedAlarmLease := ref.NewAlarmLease(ref.NewAlarmRef("B", "B-007", "Alarm-B-007"), "AA000000-000B-4000-000B-000000000007", s.p.Now(), "70cb3dc4-cb83-4f44-92d0-07b9c59ec36d")
		_, err := s.p.GetLeasedAlarm(ctx, unleaedAlarmLease)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})

	t.Run("returns ErrNoAlarm if alarm's lease belongs to others", func(t *testing.T) {
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

		// Create a fake lease with the same alarm ID but different lease ID
		fakeLease := ref.NewAlarmLease(lease.AlarmRef(), lease.Key(), lease.DueTime(), "05ac8871-02f3-4e02-b98d-a9ec231de084")

		// Try to get the alarm with the wrong lease ID
		_, err = s.p.GetLeasedAlarm(ctx, fakeLease)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})

	t.Run("returns ErrNoAlarm if alarm's lease has expired", func(t *testing.T) {
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

		// Try to get the alarm with the now-expired lease
		_, err = s.p.GetLeasedAlarm(ctx, lease)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})

	t.Run("returns alarm data correctly", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Fetch some alarms to create valid leases
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH1, SpecHostH2},
		})
		require.NoError(t, err)
		require.NotEmpty(t, res, "should have fetched and leased some alarms")

		// Find an alarm with known data - look for one of the active actor alarms
		targetLease := &ref.AlarmLease{}
		var found bool
		for _, lease := range res {
			key := strings.ToUpper(lease.Key())
			// ALM-A-1, ALM-A-2, ALM-A-4, ALM-B-1, ALM-B-2 should have specific data
			if key == SpecAlarmA1 || key == SpecAlarmB1 {
				targetLease = lease
				found = true
				break
			}
		}
		require.True(t, found, "should have found a known alarm with data")

		// Get the leased alarm details
		alarmRes, err := s.p.GetLeasedAlarm(ctx, targetLease)
		require.NoError(t, err)

		// Verify the alarm data matches expected values from GetSpec
		switch strings.ToUpper(targetLease.Key()) {
		case SpecAlarmA1:
			assert.Equal(t, "A", alarmRes.ActorType)
			assert.Equal(t, "A-1", alarmRes.ActorID)
			assert.Equal(t, "Alarm-A-1", alarmRes.Name)
			assert.Equal(t, []byte("active-A-1"), alarmRes.Data)
		case SpecAlarmB1:
			assert.Equal(t, "B", alarmRes.ActorType)
			assert.Equal(t, "B-1", alarmRes.ActorID)
			assert.Equal(t, "Alarm-B-1", alarmRes.Name)
			assert.Equal(t, []byte("active-B-1"), alarmRes.Data)
		}
	})

	t.Run("returns alarm with interval and TTL correctly", func(t *testing.T) {
		ctx := t.Context()

		// Create a custom test spec with an alarm that has interval and TTL
		customSpec := Spec{
			Hosts: []HostSpec{
				{HostID: SpecHostH1, Address: "127.0.0.1:4001", LastHealthAgo: 2 * time.Second},
			},
			HostActorTypes: []HostActorTypeSpec{
				{HostID: SpecHostH1, ActorType: "TestType", ActorIdleTimeout: 5 * time.Minute, ActorConcurrencyLimit: 0},
			},
			Alarms: []AlarmSpec{
				{
					AlarmID:   "30752437-a376-44a9-9156-b9cafcc052ee",
					ActorType: "TestType",
					ActorID:   "test-actor",
					Name:      "test-alarm",
					DueIn:     time.Second,
					Interval:  "PT1H",
					TTL:       24 * time.Hour,
					Data:      []byte("test-data-with-extras"),
				},
			},
		}

		// Seed with custom data
		require.NoError(t, s.p.Seed(ctx, customSpec))

		// Fetch the alarm to create a lease
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH1},
		})
		require.NoError(t, err)
		require.Len(t, res, 1, "should have fetched exactly one alarm")

		lease := res[0]

		// Get the leased alarm details
		alarmRes, err := s.p.GetLeasedAlarm(ctx, lease)
		require.NoError(t, err)

		// Verify all fields including interval and TTL
		assert.Equal(t, "TestType", alarmRes.ActorType)
		assert.Equal(t, "test-actor", alarmRes.ActorID)
		assert.Equal(t, "test-alarm", alarmRes.Name)
		assert.Equal(t, []byte("test-data-with-extras"), alarmRes.Data)
		assert.Equal(t, "PT1H", alarmRes.Interval)
		assert.NotNil(t, alarmRes.TTL)

		// TTL should be approximately 24 hours from now (allowing some tolerance for execution time)
		expectedTTL := s.p.Now().Add(24 * time.Hour)
		assert.WithinDuration(t, expectedTTL, *alarmRes.TTL, 10*time.Second, "TTL should be approximately 24 hours from now")
	})

	t.Run("handles nil data correctly", func(t *testing.T) {
		ctx := t.Context()

		// Create a custom test spec with an alarm that has no data
		customSpec := Spec{
			Hosts: []HostSpec{
				{HostID: SpecHostH1, Address: "127.0.0.1:4001", LastHealthAgo: 2 * time.Second},
			},
			HostActorTypes: []HostActorTypeSpec{
				{HostID: SpecHostH1, ActorType: "TestType", ActorIdleTimeout: 5 * time.Minute, ActorConcurrencyLimit: 0},
			},
			Alarms: []AlarmSpec{
				{
					AlarmID:   "980c9240-3300-4581-abc5-228843df55c5",
					ActorType: "TestType",
					ActorID:   "test-actor",
					Name:      "test-alarm",
					DueIn:     time.Second,
					Data:      nil, // No data
				},
			},
		}

		// Seed with custom data
		require.NoError(t, s.p.Seed(ctx, customSpec))

		// Fetch the alarm to create a lease
		res, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH1},
		})
		require.NoError(t, err)
		require.Len(t, res, 1, "should have fetched exactly one alarm")

		lease := res[0]

		// Get the leased alarm details
		alarmRes, err := s.p.GetLeasedAlarm(ctx, lease)
		require.NoError(t, err)

		// Verify data is nil
		assert.Nil(t, alarmRes.Data, "data should be nil when not set")
		assert.Empty(t, alarmRes.Interval, "interval should be empty when not set")
		assert.Nil(t, alarmRes.TTL, "TTL should be nil when not set")
	})
}
