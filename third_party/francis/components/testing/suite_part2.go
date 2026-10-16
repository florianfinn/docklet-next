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
	"testing"
	"time"
)

func (s Suite) TestUpdateActorHost(t *testing.T) {
	expectHosts := func(t *testing.T, expectedHosts HostSpecCollection, expectedActorTypes HostActorTypeSpecCollection) {
		t.Helper()
		spec, err := s.p.GetAllHosts(t.Context())
		require.NoError(t, err)
		assert.True(t, expectedHosts.Equal(spec.Hosts), "unexpected hosts: got=%v expected=%v", spec.Hosts, expectedHosts)
		assert.True(t, expectedActorTypes.Equal(spec.HostActorTypes), "unexpected actor types: got=%v expected=%v", spec.HostActorTypes, expectedActorTypes)
	}

	t.Run("update last health check only", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Register a host
		req := components.RegisterHostReq{
			Address: "192.0.2.100:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TestActor", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 5},
			},
		}
		res, err := s.p.RegisterHost(ctx, req)
		require.NoError(t, err)

		// Advance time to make host appear older
		_ = s.p.AdvanceClock(30 * time.Second) //nolint:errcheck

		// Update just the health check
		updateReq := components.UpdateActorHostReq{
			UpdateLastHealthCheck: true,
			ActorTypes:            nil, // Don't update actor types
		}
		err = s.p.UpdateActorHost(ctx, res.HostID, updateReq)
		require.NoError(t, err)

		// Verify host still exists with same actor types (health check updated internally)
		expectedHosts := HostSpecCollection{
			{HostID: res.HostID, Address: "192.0.2.100:8080"},
		}
		expectedActorTypes := HostActorTypeSpecCollection{
			{HostID: res.HostID, ActorType: "TestActor", ActorIdleTimeout: 5 * time.Minute, ActorConcurrencyLimit: 5},
		}
		expectHosts(t, expectedHosts, expectedActorTypes)
	})

	t.Run("update actor types only", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Register a host
		req := components.RegisterHostReq{
			Address: "192.0.2.100:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TestActor", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 5},
			},
		}
		res, err := s.p.RegisterHost(ctx, req)
		require.NoError(t, err)

		// Update just the actor types
		updateReq := components.UpdateActorHostReq{
			UpdateLastHealthCheck: false, // Don't update health check
			ActorTypes: []components.ActorHostType{
				{ActorType: "UpdatedActor", IdleTimeout: 10 * time.Minute, ConcurrencyLimit: 10},
				{ActorType: "AnotherActor", IdleTimeout: 3 * time.Minute, ConcurrencyLimit: 2},
			},
		}
		err = s.p.UpdateActorHost(ctx, res.HostID, updateReq)
		require.NoError(t, err)

		// Verify host exists with updated actor types
		expectedHosts := HostSpecCollection{
			{HostID: res.HostID, Address: "192.0.2.100:8080"},
		}
		expectedActorTypes := HostActorTypeSpecCollection{
			{HostID: res.HostID, ActorType: "UpdatedActor", ActorIdleTimeout: 10 * time.Minute, ActorConcurrencyLimit: 10},
			{HostID: res.HostID, ActorType: "AnotherActor", ActorIdleTimeout: 3 * time.Minute, ActorConcurrencyLimit: 2},
		}
		expectHosts(t, expectedHosts, expectedActorTypes)
	})

	t.Run("update both health check and actor types", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Register a host
		req := components.RegisterHostReq{
			Address: "192.0.2.100:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TestActor", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 5},
			},
		}
		res, err := s.p.RegisterHost(ctx, req)
		require.NoError(t, err)

		// Advance time
		_ = s.p.AdvanceClock(30 * time.Second) //nolint:errcheck

		// Update both health check and actor types
		updateReq := components.UpdateActorHostReq{
			UpdateLastHealthCheck: true,
			ActorTypes: []components.ActorHostType{
				{ActorType: "BothUpdatedActor", IdleTimeout: 15 * time.Minute, ConcurrencyLimit: 20},
			},
		}
		err = s.p.UpdateActorHost(ctx, res.HostID, updateReq)
		require.NoError(t, err)

		// Verify host exists with updated actor types and refreshed health check
		expectedHosts := HostSpecCollection{
			{HostID: res.HostID, Address: "192.0.2.100:8080"},
		}
		expectedActorTypes := HostActorTypeSpecCollection{
			{HostID: res.HostID, ActorType: "BothUpdatedActor", ActorIdleTimeout: 15 * time.Minute, ActorConcurrencyLimit: 20},
		}
		expectHosts(t, expectedHosts, expectedActorTypes)
	})

	t.Run("clear all actor types with empty slice", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Register a host with actor types
		req := components.RegisterHostReq{
			Address: "192.0.2.100:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TestActor", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 5},
				{ActorType: "AnotherActor", IdleTimeout: 3 * time.Minute, ConcurrencyLimit: 2},
			},
		}
		res, err := s.p.RegisterHost(ctx, req)
		require.NoError(t, err)

		// Update with empty, non-nil actor types slice (should clear all)
		updateReq := components.UpdateActorHostReq{
			UpdateLastHealthCheck: false,
			ActorTypes:            []components.ActorHostType{},
		}
		err = s.p.UpdateActorHost(ctx, res.HostID, updateReq)
		require.NoError(t, err)

		// Verify host exists but has no actor types
		expectedHosts := HostSpecCollection{
			{HostID: res.HostID, Address: "192.0.2.100:8080"},
		}
		expectedActorTypes := HostActorTypeSpecCollection{} // Empty
		expectHosts(t, expectedHosts, expectedActorTypes)
	})

	t.Run("returns ErrHostUnregistered if host not registered while updating last health check only", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Try to update a non-existent host - only last health check
		updateReq := components.UpdateActorHostReq{
			UpdateLastHealthCheck: true,
		}
		err := s.p.UpdateActorHost(ctx, SpecHostNonExistent, updateReq)
		require.ErrorIs(t, err, components.ErrHostUnregistered)
	})

	// A retry can repeat an attempt that committed after the caller stopped waiting
	// Providers may skip a redundant write when the last health check is already fresh enough
	// The retry budget bounds the freshness window so skipping remains within the health-check deadline
	t.Run("retry does not rewrite a fresh health check", func(t *testing.T) {
		// Reads back how long ago the seeded host was last health-checked
		lastHealthAgo := func(t *testing.T) time.Duration {
			t.Helper()
			spec, err := s.p.GetAllHosts(t.Context())
			require.NoError(t, err)
			require.Len(t, spec.Hosts, 1)
			return spec.Hosts[0].LastHealthAgo
		}

		// Use the same retry budget configured for each provider under test
		providerConfig := GetProviderConfig()
		budget := providerConfig.HealthCheckPolicy().Budget()

		t.Run("skips the write when the last health check is within the retry budget", func(t *testing.T) {
			// Seed a health check inside the freshness window so a retry can safely skip the write
			seeded := budget / 4
			require.NoError(t, s.p.Seed(t.Context(), Spec{
				Hosts: []HostSpec{
					{HostID: SpecHostH1, Address: "127.0.0.1:4001", LastHealthAgo: seeded},
				},
			}))

			err := s.p.UpdateActorHost(t.Context(), SpecHostH1, components.UpdateActorHostReq{
				UpdateLastHealthCheck: true,
				Retry:                 true,
			})
			require.NoError(t, err)

			// The stored value is untouched: had the write run, the host would have been checked just now
			assert.GreaterOrEqual(t, lastHealthAgo(t), seeded/2, "the retry should have left the existing health check in place")
		})

		t.Run("writes when the last health check predates the retry budget", func(t *testing.T) {
			// Seed a registered host whose health check predates the freshness window
			require.NoError(t, s.p.Seed(t.Context(), Spec{
				Hosts: []HostSpec{
					{HostID: SpecHostH1, Address: "127.0.0.1:4001", LastHealthAgo: budget * 2},
				},
			}))

			err := s.p.UpdateActorHost(t.Context(), SpecHostH1, components.UpdateActorHostReq{
				UpdateLastHealthCheck: true,
				Retry:                 true,
			})
			require.NoError(t, err)

			// The write ran, so the host has just been checked
			assert.Less(t, lastHealthAgo(t), budget, "the retry should have written a new health check")
		})

		t.Run("returns ErrHostUnregistered when the host is no longer registered", func(t *testing.T) {
			// The read must not report a host as healthy when the update would have rejected it
			require.NoError(t, s.p.Seed(t.Context(), Spec{}))

			err := s.p.UpdateActorHost(t.Context(), SpecHostNonExistent, components.UpdateActorHostReq{
				UpdateLastHealthCheck: true,
				Retry:                 true,
			})
			require.ErrorIs(t, err, components.ErrHostUnregistered)
		})
	})

	t.Run("returns ErrHostUnregistered if host not registered while updating actor types only", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Try to update a non-existent host - only actor types
		updateReq := components.UpdateActorHostReq{
			ActorTypes: []components.ActorHostType{
				{ActorType: "TestActor", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 5},
			},
		}
		err := s.p.UpdateActorHost(ctx, SpecHostNonExistent, updateReq)
		require.ErrorIs(t, err, components.ErrHostUnregistered)
	})

	t.Run("returns ErrHostUnregistered if host is unhealthy while updating last health check only", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Register a host
		req := components.RegisterHostReq{
			Address: "192.0.2.100:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TestActor", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 5},
			},
		}
		res, err := s.p.RegisterHost(ctx, req)
		require.NoError(t, err)

		// Advance time to make host unhealthy (beyond 1 minute health check deadline)
		_ = s.p.AdvanceClock(2 * time.Minute) //nolint:errcheck

		// Try to update the now-unhealthy host - only last health check
		updateReq := components.UpdateActorHostReq{
			UpdateLastHealthCheck: true,
			Retry:                 true,
		}
		err = s.p.UpdateActorHost(ctx, res.HostID, updateReq)
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrHostUnregistered)
	})

	t.Run("returns ErrHostUnregistered if host is unhealthy while updating actor types only", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Register a host
		req := components.RegisterHostReq{
			Address: "192.0.2.100:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TestActor", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 5},
			},
		}
		res, err := s.p.RegisterHost(ctx, req)
		require.NoError(t, err)

		// Advance time to make host unhealthy (beyond 1 minute health check deadline)
		_ = s.p.AdvanceClock(2 * time.Minute) //nolint:errcheck

		// Try to update the now-unhealthy host - only actor types
		updateReq := components.UpdateActorHostReq{
			ActorTypes: []components.ActorHostType{
				{ActorType: "UpdatedActor", IdleTimeout: 10 * time.Minute, ConcurrencyLimit: 10},
			},
		}
		err = s.p.UpdateActorHost(ctx, res.HostID, updateReq)
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrHostUnregistered)
	})
}

func (s Suite) TestUnregisterHost(t *testing.T) {
	expectHosts := func(t *testing.T, expectedHosts HostSpecCollection, expectedActorTypes HostActorTypeSpecCollection) {
		t.Helper()
		spec, err := s.p.GetAllHosts(t.Context())
		require.NoError(t, err)
		assert.True(t, expectedHosts.Equal(spec.Hosts), "unexpected hosts: got=%v expected=%v", spec.Hosts, expectedHosts)
		assert.True(t, expectedActorTypes.Equal(spec.HostActorTypes), "unexpected actor types: got=%v expected=%v", spec.HostActorTypes, expectedActorTypes)
	}

	t.Run("unregister healthy host", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Register a host
		req := components.RegisterHostReq{
			Address: "192.0.2.100:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TestActor", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 5},
				{ActorType: "AnotherActor", IdleTimeout: 3 * time.Minute, ConcurrencyLimit: 2},
			},
		}
		res, err := s.p.RegisterHost(ctx, req)
		require.NoError(t, err)

		// Verify host was registered
		spec, err := s.p.GetAllHosts(ctx)
		require.NoError(t, err)
		require.Len(t, spec.Hosts, 1, "should have one host registered")
		require.Len(t, spec.HostActorTypes, 2, "should have two actor types registered")

		// Unregister the host
		err = s.p.UnregisterHost(ctx, res.HostID, components.UnregisterHostOpts{})
		require.NoError(t, err)

		// Verify host and its actor types are gone
		expectHosts(t, HostSpecCollection{}, HostActorTypeSpecCollection{})
	})

	t.Run("returns ErrHostUnregistered if host not registered", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Try to unregister a non-existent host
		err := s.p.UnregisterHost(ctx, SpecHostNonExistent, components.UnregisterHostOpts{})
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrHostUnregistered)
	})

	t.Run("returns ErrHostUnregistered but deletes unhealthy host", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Register a host
		req := components.RegisterHostReq{
			Address: "192.0.2.100:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TestActor", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 5},
			},
		}
		res, err := s.p.RegisterHost(ctx, req)
		require.NoError(t, err)

		// Advance time to make host unhealthy (beyond 1 minute health check deadline)
		_ = s.p.AdvanceClock(2 * time.Minute) //nolint:errcheck

		// Unregister the now-unhealthy host - should return ErrHostUnregistered but still delete it
		err = s.p.UnregisterHost(ctx, res.HostID, components.UnregisterHostOpts{})
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrHostUnregistered)

		// Verify host and its actor types are still deleted despite the error
		expectHosts(t, HostSpecCollection{}, HostActorTypeSpecCollection{})
	})

	t.Run("unregister one of multiple hosts", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Register two hosts
		req1 := components.RegisterHostReq{
			Address: "192.0.2.100:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TypeA", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 5},
			},
		}
		res1, err := s.p.RegisterHost(ctx, req1)
		require.NoError(t, err)

		req2 := components.RegisterHostReq{
			Address: "192.0.2.101:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TypeB", IdleTimeout: 3 * time.Minute, ConcurrencyLimit: 2},
			},
		}
		res2, err := s.p.RegisterHost(ctx, req2)
		require.NoError(t, err)

		// Verify both hosts exist
		spec, err := s.p.GetAllHosts(ctx)
		require.NoError(t, err)
		require.Len(t, spec.Hosts, 2, "should have two hosts registered")
		require.Len(t, spec.HostActorTypes, 2, "should have two actor types registered")

		// Unregister the first host
		err = s.p.UnregisterHost(ctx, res1.HostID, components.UnregisterHostOpts{})
		require.NoError(t, err)

		// Verify only second host remains
		expectedHosts := HostSpecCollection{
			{HostID: res2.HostID, Address: "192.0.2.101:8080"},
		}
		expectedActorTypes := HostActorTypeSpecCollection{
			{HostID: res2.HostID, ActorType: "TypeB", ActorIdleTimeout: 3 * time.Minute, ActorConcurrencyLimit: 2},
		}
		expectHosts(t, expectedHosts, expectedActorTypes)
	})

	t.Run("conditional unregister leaves a registration owned by another session", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		ctx := t.Context()

		// Register a host owned by session s1
		req := components.RegisterHostReq{
			Address:   "192.0.2.100:8080",
			SessionID: "s1",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TestActor", IdleTimeout: 5 * time.Minute},
			},
		}
		res, err := s.p.RegisterHost(ctx, req)
		require.NoError(t, err)

		// A different session cannot remove it
		err = s.p.UnregisterHost(ctx, res.HostID, components.UnregisterHostOpts{SessionID: "s2"})
		require.ErrorIs(t, err, components.ErrHostSuperseded)
		spec, err := s.p.GetAllHosts(ctx)
		require.NoError(t, err)
		require.Len(t, spec.Hosts, 1, "host must survive a superseded unregister")

		// The host reattaches through session s2, which becomes the owner
		req.ExistingHostID = res.HostID
		req.SessionID = "s2"
		reattachRes, err := s.p.RegisterHost(ctx, req)
		require.NoError(t, err)
		require.True(t, reattachRes.Reattached)
		require.Equal(t, res.HostID, reattachRes.HostID)

		// The superseded session s1 can no longer remove it
		err = s.p.UnregisterHost(ctx, res.HostID, components.UnregisterHostOpts{SessionID: "s1"})
		require.ErrorIs(t, err, components.ErrHostSuperseded)

		// The owning session can
		err = s.p.UnregisterHost(ctx, res.HostID, components.UnregisterHostOpts{SessionID: "s2"})
		require.NoError(t, err)
		expectHosts(t, HostSpecCollection{}, HostActorTypeSpecCollection{})

		// Once the host is gone, a conditional unregister reports it as unregistered
		err = s.p.UnregisterHost(ctx, res.HostID, components.UnregisterHostOpts{SessionID: "s2"})
		require.ErrorIs(t, err, components.ErrHostUnregistered)
	})
}

func (s Suite) TestDrainingHost(t *testing.T) {
	registerTwoHosts := func(t *testing.T) (draining string, other string) {
		t.Helper()

		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		// Register two hosts that both support the same actor type, with no concurrency limit
		hostIDs := make([]string, 2)
		for i := range hostIDs {
			res, err := s.p.RegisterHost(t.Context(), components.RegisterHostReq{
				Address:   fmt.Sprintf("192.168.1.%d:8080", 100+i),
				SessionID: fmt.Sprintf("s%d", i),
				ActorTypes: []components.ActorHostType{
					{ActorType: "DrainActor", IdleTimeout: 5 * time.Minute},
				},
			})
			require.NoError(t, err)
			hostIDs[i] = res.HostID
		}

		return hostIDs[0], hostIDs[1]
	}

	t.Run("new actors are never placed on a draining host", func(t *testing.T) {
		ctx := t.Context()
		drainingID, otherID := registerTwoHosts(t)

		// Mark the first host draining
		err := s.p.UpdateActorHost(ctx, drainingID, components.UpdateActorHostReq{Draining: true})
		require.NoError(t, err)

		// Every new placement lands on the other host
		for i := range 20 {
			res, err := s.p.LookupActor(ctx, ref.NewActorRef("DrainActor", fmt.Sprintf("a%d", i)), components.LookupActorOpts{})
			require.NoError(t, err)
			require.Equal(t, otherID, res.HostID, "actor a%d was placed on the draining host", i)
		}

		// A health check does not clear the draining flag
		err = s.p.UpdateActorHost(ctx, drainingID, components.UpdateActorHostReq{UpdateLastHealthCheck: true})
		require.NoError(t, err)
		res, err := s.p.LookupActor(ctx, ref.NewActorRef("DrainActor", "after-health-check"), components.LookupActorOpts{})
		require.NoError(t, err)
		require.Equal(t, otherID, res.HostID)
	})

	t.Run("no host is available when the only capable host is draining", func(t *testing.T) {
		ctx := t.Context()
		drainingID, otherID := registerTwoHosts(t)

		// Drain both hosts
		require.NoError(t, s.p.UpdateActorHost(ctx, drainingID, components.UpdateActorHostReq{Draining: true}))
		require.NoError(t, s.p.UpdateActorHost(ctx, otherID, components.UpdateActorHostReq{Draining: true}))

		_, err := s.p.LookupActor(ctx, ref.NewActorRef("DrainActor", "a1"), components.LookupActorOpts{})
		require.ErrorIs(t, err, components.ErrNoHost)
	})

	t.Run("actors already active on a draining host stay resolvable", func(t *testing.T) {
		ctx := t.Context()
		drainingID, _ := registerTwoHosts(t)

		// Place an actor on the first host, restricting the lookup to it
		aRef := ref.NewActorRef("DrainActor", "resident")
		res, err := s.p.LookupActor(ctx, aRef, components.LookupActorOpts{Hosts: []string{drainingID}})
		require.NoError(t, err)
		require.Equal(t, drainingID, res.HostID)

		// Once the host drains, the existing placement is still returned rather than replaced
		require.NoError(t, s.p.UpdateActorHost(ctx, drainingID, components.UpdateActorHostReq{Draining: true}))
		res, err = s.p.LookupActor(ctx, aRef, components.LookupActorOpts{})
		require.NoError(t, err)
		require.Equal(t, drainingID, res.HostID)
		res, err = s.p.LookupActor(ctx, aRef, components.LookupActorOpts{ActiveOnly: true})
		require.NoError(t, err)
		require.Equal(t, drainingID, res.HostID)
	})

	t.Run("reattaching clears the draining flag", func(t *testing.T) {
		ctx := t.Context()
		drainingID, otherID := registerTwoHosts(t)

		// Drain the first host, then have it reattach through a new session
		require.NoError(t, s.p.UpdateActorHost(ctx, drainingID, components.UpdateActorHostReq{Draining: true}))
		reattachRes, err := s.p.RegisterHost(ctx, components.RegisterHostReq{
			Address:        "192.0.2.100:8080",
			ExistingHostID: drainingID,
			SessionID:      "s-new",
			ActorTypes: []components.ActorHostType{
				{ActorType: "DrainActor", IdleTimeout: 5 * time.Minute},
			},
		})
		require.NoError(t, err)
		require.True(t, reattachRes.Reattached)

		// Drain the other host, so the reattached host is the only candidate for a new placement
		require.NoError(t, s.p.UpdateActorHost(ctx, otherID, components.UpdateActorHostReq{Draining: true}))
		res, err := s.p.LookupActor(ctx, ref.NewActorRef("DrainActor", "a1"), components.LookupActorOpts{})
		require.NoError(t, err)
		require.Equal(t, drainingID, res.HostID)
	})

	t.Run("marking an unknown host draining returns ErrHostUnregistered", func(t *testing.T) {
		// Seed with empty database
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		err := s.p.UpdateActorHost(t.Context(), SpecHostNonExistent, components.UpdateActorHostReq{Draining: true})
		require.ErrorIs(t, err, components.ErrHostUnregistered)
	})
}
