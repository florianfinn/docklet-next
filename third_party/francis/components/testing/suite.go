// Source: italypaleale/francis/components/testing/suite.go
// Revision: 23cd46831a543a73eec05ad034afd46245225876
// Retrieved: 2026-10-16
// License: MIT
// Deviations: Pinned provider test support restored for default tests; reserved example addresses; oversized suite split without behavior changes.

package comptesting

import (
	"github.com/italypaleale/francis/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// Suite implements a test suite for actor provider components.
type Suite struct {
	p ActorProviderTesting
}

func NewSuite(p ActorProviderTesting) *Suite {
	return &Suite{p: p}
}

func (s Suite) RunTests(t *testing.T) {
	t.Run("register host", s.TestRegisterHost)
	t.Run("update actor host", s.TestUpdateActorHost)
	t.Run("unregister host", s.TestUnregisterHost)
	t.Run("draining host", s.TestDrainingHost)
	t.Run("list hosts", s.TestListHosts)

	t.Run("lookup actor", s.TestLookupActor)
	t.Run("remove actor", s.TestRemoveActor)

	t.Run("actor state", s.TestState)
	t.Run("list actor states", s.TestListStates)
	t.Run("workflow labels", s.TestWorkflowLabels)

	t.Run("get alarm", s.TestGetAlarm)
	t.Run("set alarm", s.TestSetAlarm)
	t.Run("set and lease alarm", s.TestSetAndLeaseAlarm)
	t.Run("delete alarm", s.TestDeleteAlarm)

	t.Run("fetch alarms", s.TestFetchAlarms)
	t.Run("get leased alarm", s.TestGetLeasedAlarm)
	t.Run("renew alarm leases", s.TestRenewAlarmLeases)
	t.Run("release alarm lease", s.TestReleaseAlarmLease)
	t.Run("update leased alarm", s.TestUpdateLeasedAlarm)
	t.Run("delete leased alarm", s.TestDeleteLeasedAlarm)

	t.Run("jobs", s.TestJobs)

	t.Run("backup and restore", s.TestBackupRestore)

	t.Run("cluster admission", s.TestClusterAdmission)
}

func (s Suite) RunConcurrencyTests(t *testing.T) {
	t.Run("lookup actor", s.TestConcurrentLookupActor)
	t.Run("fetch alarms", s.TestConcurrentFetchAlarms)
	t.Run("dispatch jobs", s.TestConcurrentDispatchJobs)
}

func (s Suite) TestRegisterHost(t *testing.T) {
	expectHosts := func(t *testing.T, expectedHosts HostSpecCollection, expectedActorTypes HostActorTypeSpecCollection) {
		t.Helper()
		spec, err := s.p.GetAllHosts(t.Context())
		require.NoError(t, err)

		actualHosts := HostSpecCollection(spec.Hosts)
		actualActorTypes := HostActorTypeSpecCollection(spec.HostActorTypes)

		assert.True(t, expectedHosts.Equal(actualHosts), "unexpected host collection: got=%v expected=%v", actualHosts, expectedHosts)
		assert.True(t, expectedActorTypes.Equal(actualActorTypes), "unexpected host actor type collection: got=%v expected=%v", actualActorTypes, expectedActorTypes)
	}

	t.Run("register new host with actor types", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		req := components.RegisterHostReq{
			Address: "192.0.2.100:8080",
			ActorTypes: []components.ActorHostType{
				{
					ActorType:        "TestActor",
					IdleTimeout:      5 * time.Minute,
					ConcurrencyLimit: 10,
				},
				{
					ActorType:        "AnotherActor",
					IdleTimeout:      2 * time.Minute,
					ConcurrencyLimit: 0, // unlimited
				},
			},
		}

		res, err := s.p.RegisterHost(ctx, req)
		require.NoError(t, err)
		assert.NotEmpty(t, res.HostID)

		expectedHosts := HostSpecCollection{
			{HostID: res.HostID, Address: "192.0.2.100:8080"},
		}
		expectedActorTypes := HostActorTypeSpecCollection{
			{HostID: res.HostID, ActorType: "TestActor", ActorIdleTimeout: 5 * time.Minute, ActorConcurrencyLimit: 10},
			{HostID: res.HostID, ActorType: "AnotherActor", ActorIdleTimeout: 2 * time.Minute, ActorConcurrencyLimit: 0},
		}
		expectHosts(t, expectedHosts, expectedActorTypes)
	})

	t.Run("cannot register host with same address if healthy", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Register first host
		req1 := components.RegisterHostReq{
			Address: "192.0.2.101:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TestActor", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 5},
			},
		}

		res1, err := s.p.RegisterHost(ctx, req1)
		require.NoError(t, err)

		// Try to register second host with same address immediately (should fail since first host is healthy)
		req2 := components.RegisterHostReq{
			Address: "192.0.2.101:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "DifferentActor", IdleTimeout: 3 * time.Minute, ConcurrencyLimit: 8},
			},
		}

		_, err = s.p.RegisterHost(ctx, req2)
		require.ErrorIs(t, err, components.ErrHostAlreadyRegistered)

		// Verify only first host still exists with original actor types
		expectedHosts := HostSpecCollection{
			{HostID: res1.HostID, Address: "192.0.2.101:8080"},
		}
		expectedActorTypes := HostActorTypeSpecCollection{
			{HostID: res1.HostID, ActorType: "TestActor", ActorIdleTimeout: 5 * time.Minute, ActorConcurrencyLimit: 5},
		}
		expectHosts(t, expectedHosts, expectedActorTypes)
	})

	t.Run("can override unhealthy host with same address", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Register first host
		req1 := components.RegisterHostReq{
			Address: "192.0.2.102:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "OldActor", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 5},
			},
		}

		res1, err := s.p.RegisterHost(ctx, req1)
		require.NoError(t, err)

		// Make the host unhealthy by advancing clock beyond health check deadline
		_ = s.p.AdvanceClock(2 * time.Minute) // Assuming health check deadline is 1 minute//nolint:errcheck

		// Register second host with same address but different actor types
		req2 := components.RegisterHostReq{
			Address: "192.0.2.102:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "NewActor", IdleTimeout: 3 * time.Minute, ConcurrencyLimit: 8},
			},
		}

		res2, err := s.p.RegisterHost(ctx, req2)
		require.NoError(t, err)
		assert.NotEmpty(t, res2.HostID)
		assert.NotEqual(t, res1.HostID, res2.HostID, "should get new host ID")

		// Verify only new host exists with new actor types
		expectedHosts := HostSpecCollection{
			{HostID: res2.HostID, Address: "192.0.2.102:8080"},
		}
		expectedActorTypes := HostActorTypeSpecCollection{
			{HostID: res2.HostID, ActorType: "NewActor", ActorIdleTimeout: 3 * time.Minute, ActorConcurrencyLimit: 8},
		}
		expectHosts(t, expectedHosts, expectedActorTypes)
	})

	t.Run("register host with no actor types", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		req := components.RegisterHostReq{
			Address:    "192.0.2.103:8080",
			ActorTypes: []components.ActorHostType{}, // empty slice
		}

		res, err := s.p.RegisterHost(ctx, req)
		require.NoError(t, err)
		assert.NotEmpty(t, res.HostID)

		expectedHosts := HostSpecCollection{
			{HostID: res.HostID, Address: "192.0.2.103:8080"},
		}
		expectedActorTypes := HostActorTypeSpecCollection{} // empty
		expectHosts(t, expectedHosts, expectedActorTypes)
	})

	t.Run("unhealthy hosts and their actor types are cleaned up", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Register multiple hosts with different actor types
		req1 := components.RegisterHostReq{
			Address: "192.0.2.104:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TypeA", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 10},
				{ActorType: "TypeB", IdleTimeout: 3 * time.Minute, ConcurrencyLimit: 5},
			},
		}
		_, err := s.p.RegisterHost(ctx, req1)
		require.NoError(t, err)

		req2 := components.RegisterHostReq{
			Address: "192.0.2.105:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TypeC", IdleTimeout: 2 * time.Minute, ConcurrencyLimit: 0},
			},
		}
		_, err = s.p.RegisterHost(ctx, req2)
		require.NoError(t, err)

		// Verify both hosts and all actor types exist
		spec, err := s.p.GetAllHosts(ctx)
		require.NoError(t, err)
		assert.Len(t, spec.Hosts, 2, "should have two hosts")
		assert.Len(t, spec.HostActorTypes, 3, "should have three actor types total")

		// Advance time to make hosts unhealthy (beyond 1 minute health check deadline)
		_ = s.p.AdvanceClock(2 * time.Minute) //nolint:errcheck

		// Register a new host - this should clean up all unhealthy hosts
		req3 := components.RegisterHostReq{
			Address: "192.0.2.106:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TypeD", IdleTimeout: 4 * time.Minute, ConcurrencyLimit: 2},
			},
		}
		res3, err := s.p.RegisterHost(ctx, req3)
		require.NoError(t, err)

		// Verify old unhealthy hosts and their actor types are gone, only new host remains
		expectedHosts := HostSpecCollection{
			{HostID: res3.HostID, Address: "192.0.2.106:8080"},
		}
		expectedActorTypes := HostActorTypeSpecCollection{
			{HostID: res3.HostID, ActorType: "TypeD", ActorIdleTimeout: 4 * time.Minute, ActorConcurrencyLimit: 2},
		}
		expectHosts(t, expectedHosts, expectedActorTypes)
	})

	// Reattachment: a reconnecting host can reclaim its registration by its previous host ID

	const (
		reattachHostA = "aaaaaaaa-0000-4000-8000-000000000001"
		reattachHostB = "bbbbbbbb-0000-4000-8000-000000000002"
		reattachHostX = "99999999-0000-4000-8000-00000000ffff"
	)

	t.Run("reattach refreshes an existing registration and preserves active actors", func(t *testing.T) {
		require.NoError(t, s.p.Seed(t.Context(), Spec{
			Hosts: HostSpecCollection{
				{HostID: reattachHostA, Address: "192.0.2.110:8080", LastHealthAgo: 2 * time.Second},
			},
			HostActorTypes: HostActorTypeSpecCollection{
				{HostID: reattachHostA, ActorType: "OldType", ActorIdleTimeout: 5 * time.Minute, ActorConcurrencyLimit: 5},
			},
			ActiveActors: []ActiveActorSpec{
				{ActorType: "OldType", ActorID: "actor-1", HostID: reattachHostA, ActorIdleTimeout: 5 * time.Minute, ActivationAgo: time.Second},
			},
		}))

		// Reattach with the same host ID, a changed address, and a replaced actor type set
		res, err := s.p.RegisterHost(t.Context(), components.RegisterHostReq{
			ExistingHostID: reattachHostA,
			Address:        "192.0.2.111:9090",
			ActorTypes: []components.ActorHostType{
				{ActorType: "NewType", IdleTimeout: 3 * time.Minute, ConcurrencyLimit: 8},
			},
		})
		require.NoError(t, err)
		assert.True(t, res.Reattached, "should report a reattachment")
		assert.Equal(t, reattachHostA, res.HostID, "should keep the same host ID")

		expectedHosts := HostSpecCollection{
			{HostID: reattachHostA, Address: "192.0.2.111:9090"},
		}
		expectedActorTypes := HostActorTypeSpecCollection{
			{HostID: reattachHostA, ActorType: "NewType", ActorIdleTimeout: 3 * time.Minute, ActorConcurrencyLimit: 8},
		}
		expectHosts(t, expectedHosts, expectedActorTypes)

		// The active actor is preserved because the host row is updated in place rather than recreated
		spec, err := s.p.GetAllHosts(t.Context())
		require.NoError(t, err)
		assert.Len(t, spec.ActiveActors, 1, "active actor should survive reattachment")
	})

	t.Run("reattach to an expired registration mints a new host ID", func(t *testing.T) {
		// Seed a host whose last health check is already older than the 1m deadline
		require.NoError(t, s.p.Seed(t.Context(), Spec{
			Hosts: HostSpecCollection{
				{HostID: reattachHostA, Address: "192.0.2.112:8080", LastHealthAgo: 90 * time.Second},
			},
			HostActorTypes: HostActorTypeSpecCollection{
				{HostID: reattachHostA, ActorType: "T", ActorIdleTimeout: 5 * time.Minute, ActorConcurrencyLimit: 1},
			},
		}))

		res, err := s.p.RegisterHost(t.Context(), components.RegisterHostReq{
			ExistingHostID: reattachHostA,
			Address:        "192.0.2.112:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "T", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 1},
			},
		})
		require.NoError(t, err)

		// Once a registration has expired the cluster has written that host off and may have placed its actors elsewhere, so it must not be reclaimable
		// The new identity, reported through Reattached, is what tells the host to drop whatever it was still holding
		assert.False(t, res.Reattached, "an expired registration must not be reattachable")
		assert.NotEqual(t, reattachHostA, res.HostID, "should mint a new host ID rather than resume the expired one")
		assert.NotEmpty(t, res.HostID)

		// The new registration is healthy and holds the address, so a fresh registration at the same address must fail
		_, err = s.p.RegisterHost(t.Context(), components.RegisterHostReq{
			Address: "192.0.2.112:8080",
		})
		require.ErrorIs(t, err, components.ErrHostAlreadyRegistered)
	})

	t.Run("reattach refreshes a live registration in place", func(t *testing.T) {
		// Seed a host whose last health check is still within the 1m deadline, as it would be right after a runtime failover
		require.NoError(t, s.p.Seed(t.Context(), Spec{
			Hosts: HostSpecCollection{
				{HostID: reattachHostA, Address: "192.0.2.112:8080", LastHealthAgo: 30 * time.Second},
			},
			HostActorTypes: HostActorTypeSpecCollection{
				{HostID: reattachHostA, ActorType: "T", ActorIdleTimeout: 5 * time.Minute, ActorConcurrencyLimit: 1},
			},
		}))

		res, err := s.p.RegisterHost(t.Context(), components.RegisterHostReq{
			ExistingHostID: reattachHostA,
			Address:        "192.0.2.112:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "T", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 1},
			},
		})
		require.NoError(t, err)
		assert.True(t, res.Reattached, "a host whose registration is still live keeps its identity")
		assert.Equal(t, reattachHostA, res.HostID)
	})

	t.Run("reattach to an unknown host creates a new registration", func(t *testing.T) {
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		res, err := s.p.RegisterHost(t.Context(), components.RegisterHostReq{
			ExistingHostID: reattachHostX,
			Address:        "192.0.2.113:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "T", IdleTimeout: time.Minute, ConcurrencyLimit: 0},
			},
		})
		require.NoError(t, err)
		assert.False(t, res.Reattached, "should not report a reattachment for an unknown host")
		assert.NotEmpty(t, res.HostID)
		assert.NotEqual(t, reattachHostX, res.HostID, "should mint a new host ID rather than adopt the unknown one")

		expectedHosts := HostSpecCollection{
			{HostID: res.HostID, Address: "192.0.2.113:8080"},
		}
		expectedActorTypes := HostActorTypeSpecCollection{
			{HostID: res.HostID, ActorType: "T", ActorIdleTimeout: time.Minute, ActorConcurrencyLimit: 0},
		}
		expectHosts(t, expectedHosts, expectedActorTypes)
	})

	t.Run("reattach to a garbage-collected host creates a new registration", func(t *testing.T) {
		// Seed a stale host, then let a fresh registration garbage-collect it
		require.NoError(t, s.p.Seed(t.Context(), Spec{
			Hosts: HostSpecCollection{
				{HostID: reattachHostA, Address: "192.0.2.114:8080", LastHealthAgo: 90 * time.Second},
			},
			HostActorTypes: HostActorTypeSpecCollection{
				{HostID: reattachHostA, ActorType: "T", ActorIdleTimeout: time.Minute, ActorConcurrencyLimit: 0},
			},
		}))

		// A fresh registration at a different address cleans up the unhealthy host
		_, err := s.p.RegisterHost(t.Context(), components.RegisterHostReq{
			Address: "192.0.2.115:8080",
		})
		require.NoError(t, err)

		// Reattaching to the now-removed host finds nothing and creates a brand-new registration
		res, err := s.p.RegisterHost(t.Context(), components.RegisterHostReq{
			ExistingHostID: reattachHostA,
			Address:        "192.0.2.114:8080",
		})
		require.NoError(t, err)
		assert.False(t, res.Reattached)
		assert.NotEqual(t, reattachHostA, res.HostID, "the previous registration was gone, so a new ID is minted")
	})

	t.Run("reattach fails when another healthy host holds the address", func(t *testing.T) {
		require.NoError(t, s.p.Seed(t.Context(), Spec{
			Hosts: HostSpecCollection{
				{HostID: reattachHostA, Address: "192.0.2.116:8080", LastHealthAgo: 2 * time.Second},
				{HostID: reattachHostB, Address: "192.0.2.117:8080", LastHealthAgo: 2 * time.Second},
			},
		}))

		// Attempt to reattach host A onto host B's address
		_, err := s.p.RegisterHost(t.Context(), components.RegisterHostReq{
			ExistingHostID: reattachHostA,
			Address:        "192.0.2.117:8080",
		})
		require.ErrorIs(t, err, components.ErrHostAlreadyRegistered)

		// Nothing changed: both hosts keep their original addresses
		expectedHosts := HostSpecCollection{
			{HostID: reattachHostA, Address: "192.0.2.116:8080"},
			{HostID: reattachHostB, Address: "192.0.2.117:8080"},
		}
		expectHosts(t, expectedHosts, HostActorTypeSpecCollection{})
	})

	t.Run("reattach with no actor types clears existing ones", func(t *testing.T) {
		require.NoError(t, s.p.Seed(t.Context(), Spec{
			Hosts: HostSpecCollection{
				{HostID: reattachHostA, Address: "192.0.2.118:8080", LastHealthAgo: 2 * time.Second},
			},
			HostActorTypes: HostActorTypeSpecCollection{
				{HostID: reattachHostA, ActorType: "T", ActorIdleTimeout: time.Minute, ActorConcurrencyLimit: 0},
			},
		}))

		res, err := s.p.RegisterHost(t.Context(), components.RegisterHostReq{
			ExistingHostID: reattachHostA,
			Address:        "192.0.2.118:8080",
			ActorTypes:     []components.ActorHostType{},
		})
		require.NoError(t, err)
		assert.True(t, res.Reattached)

		expectedHosts := HostSpecCollection{
			{HostID: reattachHostA, Address: "192.0.2.118:8080"},
		}
		expectHosts(t, expectedHosts, HostActorTypeSpecCollection{})
	})
}
