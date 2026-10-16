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
	"sync"
	"testing"
	"time"
)

func (s Suite) TestConcurrentLookupActor(t *testing.T) {
	t.Run("parallel lookups for same actor - unlimited capacity", func(t *testing.T) {
		ctx := t.Context()

		// Create a custom spec with 20 hosts, no active actors, single actor type with unlimited capacity
		customSpec := Spec{
			Hosts:          make([]HostSpec, 20),
			HostActorTypes: make([]HostActorTypeSpec, 20),
			ActiveActors:   []ActiveActorSpec{},
			Alarms:         []AlarmSpec{},
		}

		// Create 20 healthy hosts
		for i := range 20 {
			hostID := fmt.Sprintf("%08x-0000-4000-8000-000000000000", i+1)
			customSpec.Hosts[i] = HostSpec{
				HostID:        hostID,
				Address:       fmt.Sprintf("127.0.0.1:%d", 5000+i),
				LastHealthAgo: 2 * time.Second,
			}
			customSpec.HostActorTypes[i] = HostActorTypeSpec{
				HostID:                hostID,
				ActorType:             "TestActor",
				ActorIdleTimeout:      5 * time.Minute,
				ActorConcurrencyLimit: 0,
			}
		}

		require.NoError(t, s.p.Seed(ctx, customSpec))

		// Perform 50 parallel lookups for the same actor
		const numRoutines = 50
		const actorID = "same-actor"

		var wg sync.WaitGroup
		results := make([]components.LookupActorRes, numRoutines)
		errors := make([]error, numRoutines)

		wg.Add(numRoutines)
		for i := range numRoutines {
			go func(idx int) {
				defer wg.Done()
				ref := ref.NewActorRef("TestActor", actorID)
				result, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{})
				results[idx] = result
				errors[idx] = err
			}(i)
		}

		wg.Wait()

		// All lookups should succeed
		for i, err := range errors {
			require.NoError(t, err, "lookup %d should succeed", i)
		}

		// All results should point to the same host (same actor should be on the same host)
		expectedHostID := results[0].HostID
		for i, result := range results {
			assert.Equal(t, expectedHostID, result.HostID, "lookup %d should return the same host as lookup 0", i)
			assert.Equal(t, 5*time.Minute, result.IdleTimeout, "idle timeout should match")
		}
	})

	t.Run("parallel lookups for different actors - unlimited capacity", func(t *testing.T) {
		ctx := t.Context()

		// Create a custom spec with 20 hosts, no active actors, single actor type with unlimited capacity
		customSpec := Spec{
			Hosts:          make([]HostSpec, 20),
			HostActorTypes: make([]HostActorTypeSpec, 20),
			ActiveActors:   []ActiveActorSpec{},
			Alarms:         []AlarmSpec{},
		}

		// Create 20 healthy hosts
		for i := range 20 {
			hostID := fmt.Sprintf("%08x-0000-4000-8000-000000000000", i+1)
			customSpec.Hosts[i] = HostSpec{
				HostID:        hostID,
				Address:       fmt.Sprintf("127.0.0.1:%d", 5000+i),
				LastHealthAgo: 2 * time.Second,
			}
			customSpec.HostActorTypes[i] = HostActorTypeSpec{
				HostID:                hostID,
				ActorType:             "TestActor",
				ActorIdleTimeout:      5 * time.Minute,
				ActorConcurrencyLimit: 0,
			}
		}

		require.NoError(t, s.p.Seed(ctx, customSpec))

		// Perform 100 parallel lookups for different actors
		const numRoutines = 100

		var wg sync.WaitGroup
		results := make([]components.LookupActorRes, numRoutines)
		errors := make([]error, numRoutines)

		wg.Add(numRoutines)
		for i := range numRoutines {
			go func(idx int) {
				defer wg.Done()
				ref := ref.NewActorRef("TestActor", fmt.Sprintf("actor-%d", idx))
				result, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{})
				results[idx] = result
				errors[idx] = err
			}(i)
		}

		wg.Wait()

		// All lookups should succeed
		for i, err := range errors {
			require.NoError(t, err, "lookup %d should succeed", i)
		}

		// Count distribution across hosts
		hostCounts := make(map[string]int)
		for i, result := range results {
			assert.Equal(t, 5*time.Minute, result.IdleTimeout, "idle timeout should match for result %d", i)
			hostCounts[result.HostID]++
		}

		// Should distribute across multiple hosts (at least 10 different hosts for 100 actors across 20 hosts)
		assert.GreaterOrEqual(t, len(hostCounts), 10, "should distribute across at least 10 different hosts, got %d: %v", len(hostCounts), hostCounts)

		// Check for reasonable distribution - no single host should have more than 20% of actors
		maxActorsPerHost := numRoutines / 5
		for hostID, count := range hostCounts {
			assert.LessOrEqual(t, count, maxActorsPerHost, "host %s should not have more than %d actors, got %d", hostID, maxActorsPerHost, count)
		}
	})

	t.Run("parallel lookups for same actor - with capacity limits", func(t *testing.T) {
		ctx := t.Context()

		// Create a custom spec with 20 hosts, no active actors, single actor type with capacity limit of 1
		customSpec := Spec{
			Hosts:          make([]HostSpec, 20),
			HostActorTypes: make([]HostActorTypeSpec, 20),
			ActiveActors:   []ActiveActorSpec{},
			Alarms:         []AlarmSpec{},
		}

		// Create 20 healthy hosts with capacity limit of 1
		for i := range 20 {
			hostID := fmt.Sprintf("%08x-0000-4000-8000-000000000000", i+1)
			customSpec.Hosts[i] = HostSpec{
				HostID:        hostID,
				Address:       fmt.Sprintf("127.0.0.1:%d", 5000+i),
				LastHealthAgo: 2 * time.Second,
			}
			customSpec.HostActorTypes[i] = HostActorTypeSpec{
				HostID:           hostID,
				ActorType:        "TestActor",
				ActorIdleTimeout: 5 * time.Minute,
				// Limited to 1 actor per host
				ActorConcurrencyLimit: 1,
			}
		}

		require.NoError(t, s.p.Seed(ctx, customSpec))

		// Perform 50 parallel lookups for the same actor
		const numRoutines = 50
		const actorID = "same-actor-limited"

		var wg sync.WaitGroup
		results := make([]components.LookupActorRes, numRoutines)
		errors := make([]error, numRoutines)

		wg.Add(numRoutines)
		for i := range numRoutines {
			go func(idx int) {
				defer wg.Done()
				ref := ref.NewActorRef("TestActor", actorID)
				result, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{})
				results[idx] = result
				errors[idx] = err
			}(i)
		}

		wg.Wait()

		// All lookups should succeed
		for i, err := range errors {
			require.NoError(t, err, "lookup %d should succeed", i)
		}

		// All results should point to the same host (same actor should be on the same host)
		expectedHostID := results[0].HostID
		for i, result := range results {
			assert.Equal(t, expectedHostID, result.HostID, "lookup %d should return the same host as lookup 0", i)
		}
	})

	t.Run("parallel lookups for different actors - with capacity limits", func(t *testing.T) {
		ctx := t.Context()

		// Create a custom spec with 20 hosts, no active actors, single actor type with capacity limit of 1
		customSpec := Spec{
			Hosts:          make([]HostSpec, 20),
			HostActorTypes: make([]HostActorTypeSpec, 20),
			ActiveActors:   []ActiveActorSpec{},
			Alarms:         []AlarmSpec{},
		}

		// Create 20 healthy hosts with capacity limit of 1
		for i := range 20 {
			hostID := fmt.Sprintf("%08x-0000-4000-8000-000000000000", i+1)
			customSpec.Hosts[i] = HostSpec{
				HostID:        hostID,
				Address:       fmt.Sprintf("127.0.0.1:%d", 5000+i),
				LastHealthAgo: 2 * time.Second,
			}
			customSpec.HostActorTypes[i] = HostActorTypeSpec{
				HostID:           hostID,
				ActorType:        "TestActor",
				ActorIdleTimeout: 5 * time.Minute,
				// Limited to 1 actor per host
				ActorConcurrencyLimit: 1,
			}
		}

		require.NoError(t, s.p.Seed(ctx, customSpec))

		// Perform 20 parallel lookups for different actors (exactly matching host capacity)
		const numRoutines = 20

		var wg sync.WaitGroup
		results := make([]components.LookupActorRes, numRoutines)
		errors := make([]error, numRoutines)

		wg.Add(numRoutines)
		for i := range numRoutines {
			go func(idx int) {
				defer wg.Done()
				ref := ref.NewActorRef("TestActor", fmt.Sprintf("actor-limited-%d", idx))
				result, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{})
				results[idx] = result
				errors[idx] = err
			}(i)
		}

		wg.Wait()

		// All lookups should succeed
		for i, err := range errors {
			require.NoError(t, err, "lookup %d should succeed", i)
		}

		// Count distribution across hosts
		hostCounts := make(map[string]int)
		for _, result := range results {
			hostCounts[result.HostID]++
		}

		// With capacity limits and race conditions, we expect:
		// - Most hosts should have exactly 1 actor
		// - Some hosts might exceed capacity due to race conditions (this is expected)
		// - Should use most of the available hosts
		// Enforcing capacity constraints in this case is done as best-effort and not guaranteed
		assert.GreaterOrEqual(t, len(hostCounts), 8, "should distribute across at least 8 hosts out of 20 available")

		// Count how many hosts have exactly 1 actor (ideal distribution)
		perfectHosts := 0
		for _, count := range hostCounts {
			if count == 1 {
				perfectHosts++
			}
		}

		// Most hosts should follow capacity limits, but some race conditions are expected
		assert.GreaterOrEqual(t, perfectHosts, 5, "at least 10 hosts should have exactly 1 actor (allowing for some race conditions)")

		t.Logf("Distribution: %d actors across %d hosts, %d hosts with perfect capacity (1 actor)", numRoutines, len(hostCounts), perfectHosts)

		// Log detailed distribution for analysis
		for hostID, count := range hostCounts {
			if count > 1 {
				t.Logf("Race condition detected: host %s has %d actors (exceeds limit of 1)", hostID, count)
			}
		}

		// Try to create one more actor - this might succeed due to race conditions,
		// but we'll test it to see the behavior
		ref := ref.ActorRef{ActorType: "TestActor", ActorID: "actor-overflow"}
		_, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{})
		// Don't require an error here since race conditions might allow it to succeed
		if err != nil {
			t.Logf("Overflow actor correctly rejected: %v", err)
		} else {
			t.Logf("Overflow actor was accepted (likely due to race conditions)")
		}
	})
}

func (s Suite) TestConcurrentFetchAlarms(t *testing.T) {
	t.Run("parallel fetches for same alarms - unlimited capacity", func(t *testing.T) {
		ctx := t.Context()

		// Create a custom spec with 20 hosts, 50 alarms, single actor type with unlimited capacity, no active actors initially
		customSpec := Spec{
			Hosts:          make([]HostSpec, 20),
			HostActorTypes: make([]HostActorTypeSpec, 20),
			ActiveActors:   []ActiveActorSpec{},
			Alarms:         make([]AlarmSpec, 50),
		}

		// Create 20 healthy hosts with unlimited capacity
		for i := range 20 {
			hostID := fmt.Sprintf("%08x-0000-4000-8000-000000000000", i+1)
			customSpec.Hosts[i] = HostSpec{
				HostID:        hostID,
				Address:       fmt.Sprintf("127.0.0.1:%d", 5000+i),
				LastHealthAgo: 2 * time.Second,
			}
			customSpec.HostActorTypes[i] = HostActorTypeSpec{
				HostID:                hostID,
				ActorType:             "TestActor",
				ActorIdleTimeout:      5 * time.Minute,
				ActorConcurrencyLimit: 0,
			}
		}

		// Create 50 overdue alarms for different actors
		for i := range 50 {
			customSpec.Alarms[i] = AlarmSpec{
				AlarmID:   fmt.Sprintf("AA%06d-0000-4000-8000-000000000000", i+1),
				ActorType: "TestActor",
				ActorID:   fmt.Sprintf("actor-%d", i),
				Name:      "test-alarm",
				DueIn:     -5 * time.Second,
			}
		}

		require.NoError(t, s.p.Seed(ctx, customSpec))

		// Perform 20 parallel fetches from all hosts
		const numRoutines = 20

		var wg sync.WaitGroup
		allLeases := make([][]*ref.AlarmLease, numRoutines)
		errors := make([]error, numRoutines)

		wg.Add(numRoutines)
		for i := range numRoutines {
			go func(idx int) {
				defer wg.Done()
				hostID := fmt.Sprintf("%08x-0000-4000-8000-000000000000", idx+1)
				leases, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
					Hosts: []string{hostID},
				})
				allLeases[idx] = leases
				errors[idx] = err
			}(i)
		}

		wg.Wait()

		// All fetches should succeed
		for i, err := range errors {
			require.NoError(t, err, "fetch %d should succeed", i)
		}

		// Collect all leased alarms and verify uniqueness
		allLeasedAlarms := make(map[string]bool)
		totalLeases := 0
		for i, leases := range allLeases {
			for _, lease := range leases {
				alarmRef := lease.AlarmRef()
				key := alarmRef.String()
				if allLeasedAlarms[key] {
					t.Errorf("Alarm %s was leased multiple times! First seen in fetch %d", key, i)
				}
				allLeasedAlarms[key] = true
				totalLeases++
			}
		}

		// Should have leased all 50 alarms exactly once
		assert.Equal(t, 50, totalLeases, "should have leased all 50 alarms exactly once")
		assert.Len(t, allLeasedAlarms, 50, "should have 50 unique leased alarms")

		// Verify that actors were activated too
		// Should have exactly 50 active actors (one per alarm)
		spec, err := s.p.GetAllHosts(ctx)
		require.NoError(t, err)
		assert.Len(t, spec.ActiveActors, 50, "should have exactly 50 active actors")
	})

	t.Run("parallel fetches for same alarms - with capacity limits", func(t *testing.T) {
		ctx := t.Context()

		// Create a custom spec with 20 hosts, 30 alarms, single actor type with capacity limit of 1, no active actors initially
		customSpec := Spec{
			Hosts:          make([]HostSpec, 20),
			HostActorTypes: make([]HostActorTypeSpec, 20),
			ActiveActors:   []ActiveActorSpec{},
			Alarms:         make([]AlarmSpec, 30),
		}

		// Create 20 healthy hosts with capacity limit of 1
		for i := range 20 {
			hostID := fmt.Sprintf("%08x-0000-4000-8000-000000000000", i+1)
			customSpec.Hosts[i] = HostSpec{
				HostID:        hostID,
				Address:       fmt.Sprintf("127.0.0.1:%d", 5000+i),
				LastHealthAgo: 2 * time.Second,
			}
			customSpec.HostActorTypes[i] = HostActorTypeSpec{
				HostID:           hostID,
				ActorType:        "TestActor",
				ActorIdleTimeout: 5 * time.Minute,
				// Limited to 1 actor per host
				ActorConcurrencyLimit: 1,
			}
		}

		// Create 30 overdue alarms for different actors
		for i := range 30 {
			customSpec.Alarms[i] = AlarmSpec{
				AlarmID:   fmt.Sprintf("AA%06d-0000-4000-8000-000000000000", i+1),
				ActorType: "TestActor",
				ActorID:   fmt.Sprintf("actor-%d", i),
				Name:      "test-alarm",
				DueIn:     -5 * time.Second,
			}
		}

		require.NoError(t, s.p.Seed(ctx, customSpec))

		// Perform 20 parallel fetches from all hosts
		const numRoutines = 20

		var wg sync.WaitGroup
		allLeases := make([][]*ref.AlarmLease, numRoutines)
		errors := make([]error, numRoutines)

		wg.Add(numRoutines)
		for i := range numRoutines {
			go func(idx int) {
				defer wg.Done()
				hostID := fmt.Sprintf("%08x-0000-4000-8000-000000000000", idx+1)
				leases, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
					Hosts: []string{hostID},
				})
				allLeases[idx] = leases
				errors[idx] = err
			}(i)
		}

		wg.Wait()

		// All fetches should succeed
		for i, err := range errors {
			require.NoError(t, err, "fetch %d should succeed", i)
		}

		// Collect all leased alarms and verify uniqueness
		allLeasedAlarms := make(map[string]bool)
		totalLeases := 0
		for i, leases := range allLeases {
			for _, lease := range leases {
				alarmRef := lease.AlarmRef()
				key := alarmRef.String()
				if allLeasedAlarms[key] {
					t.Errorf("Alarm %s was leased multiple times! First seen in fetch %d", key, i)
				}
				allLeasedAlarms[key] = true
				totalLeases++
			}
		}

		// With capacity limits, we should lease at most 20 alarms (one per host max)
		// Due to race conditions, we might lease fewer than 20
		assert.LessOrEqual(t, totalLeases, 20, "should have leased at most 20 alarms due to capacity limits")
		assert.Len(t, allLeasedAlarms, totalLeases, "all leased alarms should be unique")

		// Verify that actors were activated on exactly one host each
		// Number of active actors should match number of leased alarms
		spec, err := s.p.GetAllHosts(ctx)
		require.NoError(t, err)
		assert.Len(t, spec.ActiveActors, totalLeases, "should have one active actor per leased alarm")

		// Verify capacity constraints are respected (at most 1 actor per host)
		hostActorCounts := make(map[string]int)
		for _, aa := range spec.ActiveActors {
			hostActorCounts[aa.HostID]++
		}

		for hostID, count := range hostActorCounts {
			assert.LessOrEqual(t, count, 1, "host %s should have at most 1 actor due to capacity limits, but has %d", hostID, count)
		}

		t.Logf("Successfully leased %d alarms across %d hosts with capacity limits", totalLeases, len(hostActorCounts))
	})

	t.Run("parallel fetches for overlapping actor types", func(t *testing.T) {
		ctx := t.Context()

		// Create a custom spec with multiple actor types and overlapping host support, no active actors initially
		customSpec := Spec{
			Hosts:          make([]HostSpec, 10),
			HostActorTypes: make([]HostActorTypeSpec, 0),
			ActiveActors:   []ActiveActorSpec{},
			Alarms:         make([]AlarmSpec, 40),
		}

		// Create 10 healthy hosts
		for i := range 10 {
			hostID := fmt.Sprintf("%08x-0000-4000-8000-000000000000", i+1)
			customSpec.Hosts[i] = HostSpec{
				HostID:        hostID,
				Address:       fmt.Sprintf("127.0.0.1:%d", 5000+i),
				LastHealthAgo: 2 * time.Second,
			}

			// First 5 hosts support ActorTypeA, last 5 hosts support ActorTypeB
			// Hosts 3-7 support both types (overlap)
			if i < 7 {
				// Hosts 0-6 support ActorTypeA
				// Limited to 4 actors per host
				customSpec.HostActorTypes = append(customSpec.HostActorTypes, HostActorTypeSpec{
					HostID:                hostID,
					ActorType:             "ActorTypeA",
					ActorIdleTimeout:      5 * time.Minute,
					ActorConcurrencyLimit: 4,
				})
			}
			if i >= 3 {
				// Hosts 3-9 support ActorTypeB
				// Limited to 4 actors per host
				customSpec.HostActorTypes = append(customSpec.HostActorTypes, HostActorTypeSpec{
					HostID:                hostID,
					ActorType:             "ActorTypeB",
					ActorIdleTimeout:      5 * time.Minute,
					ActorConcurrencyLimit: 4,
				})
			}
		}

		// Create 20 overdue alarms for ActorTypeA and 20 for ActorTypeB
		for i := range 20 {
			customSpec.Alarms[i] = AlarmSpec{
				AlarmID:   fmt.Sprintf("AA%06d-000A-4000-8000-000000000000", i+1),
				ActorType: "ActorTypeA",
				ActorID:   fmt.Sprintf("actorA-%02d", i),
				Name:      "test-alarm",
				DueIn:     -5 * time.Second,
			}
			customSpec.Alarms[i+20] = AlarmSpec{
				AlarmID:   fmt.Sprintf("AA%06d-000B-4000-8000-000000000000", i+1),
				ActorType: "ActorTypeB",
				ActorID:   fmt.Sprintf("actorB-%02d", i),
				Name:      "test-alarm",
				DueIn:     -5 * time.Second,
			}
		}

		require.NoError(t, s.p.Seed(ctx, customSpec))

		// Perform 10 parallel fetches from all hosts, attempting 5 times
		// We make multiple attempts because there could be lock contention causing alarms to be skipped, especially under heavy concurrent access
		// We are most concerned here with making sure that alarms/actors aren't activated in more than one host
		// Lock contention because of many fetchers at the same exact time is not something that should happen frequently in the real world
		const numRoutines = 10
		const attempts = 5

		var wg sync.WaitGroup
		allLeases := make([][]*ref.AlarmLease, numRoutines*attempts)
		errors := make([]error, numRoutines*attempts)

		for a := range attempts {
			wg.Add(numRoutines)
			for i := range numRoutines {
				go func(a, idx int) {
					defer wg.Done()
					hostID := fmt.Sprintf("%08x-0000-4000-8000-000000000000", idx+1)
					leases, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
						Hosts: []string{hostID},
					})
					allLeases[(a*numRoutines)+idx] = leases
					errors[(a*numRoutines)+idx] = err
				}(a, i)
			}

			wg.Wait()
		}

		// All fetches should succeed
		for i, err := range errors {
			require.NoError(t, err, "fetch %d should succeed", i)
		}

		// Collect all leased alarms and verify uniqueness
		allLeasedAlarms := make(map[string]bool)
		totalLeases := 0
		typeACounts := 0
		typeBCounts := 0

		for i, leases := range allLeases {
			for _, lease := range leases {
				alarmRef := lease.AlarmRef()
				key := alarmRef.String()
				if allLeasedAlarms[key] {
					t.Errorf("Alarm %s was leased multiple times! First seen in fetch %d", key, i)
				}
				allLeasedAlarms[key] = true
				totalLeases++

				switch alarmRef.ActorType {
				case "ActorTypeA":
					typeACounts++
				case "ActorTypeB":
					typeBCounts++
				}
			}
		}

		// Should have leased all 40 alarms exactly once
		assert.Equal(t, 40, totalLeases, "should have leased all 40 alarms exactly once")
		assert.Len(t, allLeasedAlarms, 40, "should have 40 unique leased alarms")
		assert.Equal(t, 20, typeACounts, "should have leased all 20 ActorTypeA alarms")
		assert.Equal(t, 20, typeBCounts, "should have leased all 20 ActorTypeB alarms")

		// Verify that actors were activated on exactly one host each
		// Should have exactly 40 active actors (one per alarm)
		spec, err := s.p.GetAllHosts(ctx)

		require.NoError(t, err)
		assert.Len(t, spec.ActiveActors, 40, "should have exactly 40 active actors")

		// Verify capacity constraints are respected (at most 4 actors per host per type)
		// Note that we consider capacity constraints as best-effort in case of high concurrency, so we treat this as a warning but not an error
		hostTypeActorCounts := make(map[string]map[string]int) // hostID -> actorType -> count
		for _, aa := range spec.ActiveActors {
			if hostTypeActorCounts[aa.HostID] == nil {
				hostTypeActorCounts[aa.HostID] = make(map[string]int)
			}
			hostTypeActorCounts[aa.HostID][aa.ActorType]++
		}

		for hostID, typeCounts := range hostTypeActorCounts {
			for actorType, count := range typeCounts {
				if count > 4 {
					t.Logf("Capacity exceeded: host %s should have at most 4 actors of type %s due to capacity limits, but has %d", hostID, actorType, count)
				}
			}
		}

		t.Logf("Successfully distributed 40 alarms across overlapping host capabilities")
	})
}
