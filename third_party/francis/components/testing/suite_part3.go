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

func (s Suite) TestListHosts(t *testing.T) {
	// hostIDs returns the set of host IDs in the result
	hostIDs := func(hosts []components.HostInfo) []string {
		ids := make([]string, len(hosts))
		for i, h := range hosts {
			ids[i] = h.HostID
		}
		return ids
	}

	t.Run("returns empty slice when no hosts are registered", func(t *testing.T) {
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))

		hosts, err := s.p.ListHosts(t.Context())
		require.NoError(t, err)
		assert.Empty(t, hosts)
	})

	t.Run("returns only registered and healthy hosts", func(t *testing.T) {
		require.NoError(t, s.p.Seed(t.Context(), GetSpec()))

		hosts, err := s.p.ListHosts(t.Context())
		require.NoError(t, err)

		// From GetSpec: H1, H2, H3, H4, H7, H8 are healthy, while H5, H6, H9 are unhealthy
		got := hostIDs(hosts)
		expected := []string{SpecHostH1, SpecHostH2, SpecHostH3, SpecHostH4, SpecHostH7, SpecHostH8}
		assert.ElementsMatch(t, expected, got, "should return exactly the healthy hosts")

		// Unhealthy hosts must never be reported
		assert.NotContains(t, got, SpecHostH5)
		assert.NotContains(t, got, SpecHostH6)
		assert.NotContains(t, got, SpecHostH9)
	})

	t.Run("includes host ID, address, and last health check for each host", func(t *testing.T) {
		require.NoError(t, s.p.Seed(t.Context(), GetSpec()))

		hosts, err := s.p.ListHosts(t.Context())
		require.NoError(t, err)

		byID := make(map[string]components.HostInfo, len(hosts))
		for _, h := range hosts {
			byID[h.HostID] = h
		}

		expectedAddrs := map[string]string{
			SpecHostH1: "127.0.0.1:4001",
			SpecHostH2: "127.0.0.1:4002",
			SpecHostH3: "127.0.0.1:4003",
			SpecHostH4: "127.0.0.1:4004",
			SpecHostH7: "127.0.0.1:4007",
			SpecHostH8: "127.0.0.1:4008",
		}

		now := s.p.Now()
		deadline := GetProviderConfig().HostHealthCheckDeadline
		for id, addr := range expectedAddrs {
			h, ok := byID[id]
			require.True(t, ok, "host %s should be present", id)
			assert.Equal(t, id, h.HostID)
			assert.Equal(t, addr, h.Address)

			// The last health check must be set, in the past, and recent enough that the host is healthy
			assert.False(t, h.LastHealthCheck.IsZero(), "last health check should be set for host %s", id)
			assert.False(t, h.LastHealthCheck.After(now), "last health check for host %s should not be in the future", id)
			assert.WithinDuration(t, now, h.LastHealthCheck, deadline, "last health check for host %s should be within the health deadline", id)
		}
	})

	t.Run("reflects a newly registered host", func(t *testing.T) {
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))
		ctx := t.Context()

		res, err := s.p.RegisterHost(ctx, components.RegisterHostReq{
			Address: "203.0.113.6:8080",
			ActorTypes: []components.ActorHostType{
				{ActorType: "TestActor", IdleTimeout: 5 * time.Minute, ConcurrencyLimit: 1},
			},
		})
		require.NoError(t, err)

		hosts, err := s.p.ListHosts(ctx)
		require.NoError(t, err)
		require.Len(t, hosts, 1)
		assert.Equal(t, res.HostID, hosts[0].HostID)
		assert.Equal(t, "203.0.113.6:8080", hosts[0].Address)
	})

	t.Run("returns all healthy hosts when multiple are registered", func(t *testing.T) {
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))
		ctx := t.Context()

		addrs := []string{"203.0.113.7:8080", "203.0.113.8:8080", "203.0.113.9:8080"}
		ids := make([]string, len(addrs))
		for i, addr := range addrs {
			res, err := s.p.RegisterHost(ctx, components.RegisterHostReq{Address: addr})
			require.NoError(t, err)
			ids[i] = res.HostID
		}

		hosts, err := s.p.ListHosts(ctx)
		require.NoError(t, err)
		assert.ElementsMatch(t, ids, hostIDs(hosts))
	})

	t.Run("excludes a host once it becomes unhealthy", func(t *testing.T) {
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))
		ctx := t.Context()

		_, err := s.p.RegisterHost(ctx, components.RegisterHostReq{Address: "203.0.113.10:8080"})
		require.NoError(t, err)

		// The host is healthy right after registration
		hosts, err := s.p.ListHosts(ctx)
		require.NoError(t, err)
		require.Len(t, hosts, 1)

		// Advance the clock beyond the health check deadline (1 minute)
		_ = s.p.AdvanceClock(2 * time.Minute) //nolint:errcheck

		hosts, err = s.p.ListHosts(ctx)
		require.NoError(t, err)
		assert.Empty(t, hosts, "an unhealthy host should not be listed")
	})

	t.Run("excludes an unregistered host", func(t *testing.T) {
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))
		ctx := t.Context()

		res1, err := s.p.RegisterHost(ctx, components.RegisterHostReq{Address: "203.0.113.11:8080"})
		require.NoError(t, err)
		res2, err := s.p.RegisterHost(ctx, components.RegisterHostReq{Address: "203.0.113.12:8080"})
		require.NoError(t, err)

		err = s.p.UnregisterHost(ctx, res1.HostID, components.UnregisterHostOpts{})
		require.NoError(t, err)

		hosts, err := s.p.ListHosts(ctx)
		require.NoError(t, err)
		require.Len(t, hosts, 1)
		assert.Equal(t, res2.HostID, hosts[0].HostID)
	})

	t.Run("keeps a host listed after its health check is refreshed", func(t *testing.T) {
		require.NoError(t, s.p.Seed(t.Context(), Spec{}))
		ctx := t.Context()

		res, err := s.p.RegisterHost(ctx, components.RegisterHostReq{Address: "203.0.113.13:8080"})
		require.NoError(t, err)

		// Advance partway, then refresh the health check before the deadline
		_ = s.p.AdvanceClock(40 * time.Second) //nolint:errcheck
		err = s.p.UpdateActorHost(ctx, res.HostID, components.UpdateActorHostReq{UpdateLastHealthCheck: true})
		require.NoError(t, err)

		// Advance again, but the total since the refresh is still under the deadline
		_ = s.p.AdvanceClock(40 * time.Second) //nolint:errcheck

		hosts, err := s.p.ListHosts(ctx)
		require.NoError(t, err)
		require.Len(t, hosts, 1)
		assert.Equal(t, res.HostID, hosts[0].HostID)
	})
}

func (s Suite) TestLookupActor(t *testing.T) {
	t.Run("returns existing actor on healthy host", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Look up an existing actor that's already active on a healthy host
		// From GetSpec: B-1 is active on H1 (healthy)
		ref := ref.ActorRef{ActorType: "B", ActorID: "B-1"}
		res, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{})
		require.NoError(t, err)

		// Should return the existing host H1
		assert.Equal(t, SpecHostH1, res.HostID)
		assert.Equal(t, "127.0.0.1:4001", res.Address)
		assert.Equal(t, 5*time.Minute, res.IdleTimeout)
	})

	t.Run("creates new actor when not active", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Create multiple actors to validate they're distributed across different hosts
		seenHosts := make(map[string]bool)
		for i := range 10 { // Try up to 10 times to see distribution
			ref := ref.ActorRef{ActorType: "B", ActorID: fmt.Sprintf("B-new-%d", i)}
			res, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{})
			require.NoError(t, err)

			// Should place it on one of the healthy hosts that support B (H1, H2, or H3)
			assert.Contains(t, []string{SpecHostH1, SpecHostH2, SpecHostH3}, res.HostID)
			assert.Contains(t, []string{"127.0.0.1:4001", "127.0.0.1:4002", "127.0.0.1:4003"}, res.Address)
			assert.Equal(t, 5*time.Minute, res.IdleTimeout)

			seenHosts[res.HostID] = true

			// If we've seen more than one host, we've validated distribution
			if len(seenHosts) > 1 {
				break
			}
		}

		// Should have distributed across multiple hosts
		assert.Greater(t, len(seenHosts), 1, "actors should be distributed across multiple hosts, but only saw: %v", seenHosts)
	})

	t.Run("replaces actor on unhealthy host", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Look up an actor that exists only on unhealthy host H6
		// From GetSpec: D-1 is active on H6 (unhealthy), but D is only supported on H6
		// This should fail with ErrNoHost because D is not supported on any healthy host
		ref := ref.ActorRef{ActorType: "D", ActorID: "D-1"}
		_, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{})
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrNoHost)
	})

	t.Run("respects host restrictions on active actor - allowed host", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Look up actor B-1 which is active on H1, but restrict to only H1
		ref := ref.ActorRef{ActorType: "B", ActorID: "B-1"}
		opts := components.LookupActorOpts{Hosts: []string{SpecHostH1}}
		res, err := s.p.LookupActor(ctx, ref, opts)
		require.NoError(t, err)

		// Should return the existing actor on H1
		assert.Equal(t, SpecHostH1, res.HostID)
		assert.Equal(t, "127.0.0.1:4001", res.Address)
	})

	t.Run("respects host restrictions on active actor - disallowed host", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Look up actor B-1 which is active on H1, but restrict to only H2
		// This should return ErrNoHost because the actor is on a disallowed host
		ref := ref.ActorRef{ActorType: "B", ActorID: "B-1"}
		opts := components.LookupActorOpts{Hosts: []string{SpecHostH2}}
		_, err := s.p.LookupActor(ctx, ref, opts)
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrNoHost)
	})

	t.Run("creates new actor with host restrictions", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Create 3 actors of type C, but restrict to only H2
		// Type C has unlimited capacity so this should work
		for i := range 3 {
			ref := ref.ActorRef{ActorType: "C", ActorID: fmt.Sprintf("C-restricted-%d", i)}
			opts := components.LookupActorOpts{Hosts: []string{SpecHostH2}}
			res, err := s.p.LookupActor(ctx, ref, opts)
			require.NoError(t, err)

			// Should always place it on H2 only
			assert.Equal(t, SpecHostH2, res.HostID)
			assert.Equal(t, "127.0.0.1:4002", res.Address)
			assert.Equal(t, 5*time.Minute, res.IdleTimeout)
		}
	})

	t.Run("returns ErrNoHost when no capacity available", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Try to create a new actor of type A
		// From GetSpec: A is at capacity on both H1 (3/3) and H2 (2/2)
		ref := ref.ActorRef{ActorType: "A", ActorID: "A-new"}
		_, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{})
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrNoHost)
	})

	t.Run("creates unlimited actors on healthy hosts", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Create actors of type C (unlimited on H1 and H2) to validate distribution
		// Placement picks one of the eligible hosts at random, so the sample has to be large enough for the evenness assertion below to be about the implementation rather than about luck
		const (
			numActors  = 100
			minPerHost = numActors / 5
		)

		hostCounts := make(map[string]int)
		for i := range numActors {
			ref := ref.ActorRef{ActorType: "C", ActorID: fmt.Sprintf("C-unlimited-%d", i)}
			res, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{})
			require.NoError(t, err)

			// Should place it on one of the healthy hosts that support C (H1 or H2)
			assert.Contains(t, []string{SpecHostH1, SpecHostH2}, res.HostID)
			assert.Contains(t, []string{"127.0.0.1:4001", "127.0.0.1:4002"}, res.Address)
			assert.Equal(t, 5*time.Minute, res.IdleTimeout)

			hostCounts[res.HostID]++
		}

		// Should have distributed across both hosts
		assert.Len(t, hostCounts, 2, "should distribute across both H1 and H2")

		// Validate approximately even distribution: each host should get at least a fifth of the actors
		// With a fair pick between two hosts, either host falling below 20 out of 100 has a probability of about 3e-10, so this tolerates the randomness while still catching an implementation that stops spreading actors
		h1Count := hostCounts[SpecHostH1]
		h2Count := hostCounts[SpecHostH2]

		assert.GreaterOrEqual(t, h1Count, minPerHost, "H1 should have at least %d actors for reasonable distribution, got %d", minPerHost, h1Count)
		assert.GreaterOrEqual(t, h2Count, minPerHost, "H2 should have at least %d actors for reasonable distribution, got %d", minPerHost, h2Count)
		assert.Equal(t, numActors, h1Count+h2Count, "total should be %d actors", numActors)

		t.Logf("Distribution: H1=%d, H2=%d", h1Count, h2Count)
	})

	t.Run("ignores unhealthy hosts for new actors", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Create multiple actors of type C (unlimited capacity) to validate they never go to unhealthy hosts
		// Type C is supported on H1 and H2 (both healthy) but not on H5/H6 (unhealthy)
		const numActors = 40
		seenHosts := make(map[string]bool)
		for i := range numActors { // Try multiple times to ensure consistent behavior
			ref := ref.ActorRef{ActorType: "C", ActorID: fmt.Sprintf("C-ignore-unhealthy-%d", i)}
			res, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{})
			require.NoError(t, err)

			// Should ONLY be placed on healthy hosts H1, H2 (where C is supported)
			assert.Contains(t, []string{SpecHostH1, SpecHostH2}, res.HostID)
			assert.NotEqual(t, SpecHostH5, res.HostID) // H5 is unhealthy
			assert.NotEqual(t, SpecHostH6, res.HostID) // H6 is unhealthy
			assert.NotEqual(t, SpecHostH3, res.HostID) // H3 doesn't support C

			seenHosts[res.HostID] = true
		}

		// Should have used both healthy hosts (validation that distribution works)
		// Each placement is an independent random pick between H1 and H2, so every lookup landing on the same host has a probability of about 2e-12 with this many iterations
		assert.Len(t, seenHosts, 2, "should distribute across both healthy hosts that support C: %v", seenHosts)
	})

	t.Run("returns ErrNoHost for unsupported actor type", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Try to create an actor of type "UNSUPPORTED"
		ref := ref.ActorRef{ActorType: "UNSUPPORTED", ActorID: "unsupported-1"}
		_, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{})
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrNoHost)
	})

	t.Run("host restrictions with non-existent host", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Try to create actor with restriction to non-existent host
		ref := ref.ActorRef{ActorType: "B", ActorID: "B-nonexistent-host"}
		opts := components.LookupActorOpts{Hosts: []string{"1da70d19-ea7a-448e-934a-c03605c3d2ee"}}
		_, err := s.p.LookupActor(ctx, ref, opts)
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrNoHost)
	})

	t.Run("validates capacity tracking and exhaustion", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// From GetSpec: Type A has capacity limits:
		// H1: supports A with capacity 3, currently has 3 active (at capacity)
		// H2: supports A with capacity 2, currently has 2 active (at capacity)
		// Total capacity for A is full (5/5)

		// Verify initial state - should already be at capacity
		_, err := s.p.LookupActor(ctx, ref.ActorRef{ActorType: "A", ActorID: "A-should-fail"}, components.LookupActorOpts{})
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrNoHost, "should fail when capacity is already exhausted")

		// Get initial host state to verify capacity tracking
		spec, err := s.p.GetAllHosts(ctx)
		require.NoError(t, err)

		// Find hosts that support type A and verify their active counts
		var h1ActiveCount, h2ActiveCount int
		for _, activeActor := range spec.ActiveActors {
			if activeActor.ActorType == "A" {
				switch activeActor.HostID {
				case SpecHostH1:
					h1ActiveCount++
				case SpecHostH2:
					h2ActiveCount++
				}
			}
		}

		// Verify initial capacity usage matches expected from GetSpec
		assert.Equal(t, 3, h1ActiveCount, "H1 should have 3 active A actors")
		assert.Equal(t, 2, h2ActiveCount, "H2 should have 2 active A actors")

		// Now let's create space by using a different actor type (B) to verify capacity tracking works
		// Create several B actors to fill up some capacity on hosts that also support A
		createdActors := 0
		for i := range 10 {
			ref := ref.ActorRef{ActorType: "B", ActorID: fmt.Sprintf("B-capacity-test-%d", i)}
			res, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{})
			if err != nil {
				break // Stop if we can't create more
			}
			createdActors++

			// Verify the actor was created on a valid host
			assert.Contains(t, []string{SpecHostH1, SpecHostH2, SpecHostH3}, res.HostID)
		}

		// Verify we could create at least some B actors (B has unlimited capacity on some hosts)
		assert.Positive(t, createdActors, "should be able to create B actors since they have unlimited capacity")

		// Verify that A is still at capacity after creating B actors
		_, err = s.p.LookupActor(ctx, ref.ActorRef{ActorType: "A", ActorID: "A-still-should-fail"}, components.LookupActorOpts{})
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrNoHost, "A should still be at capacity")

		// Get final state to verify capacity tracking
		finalSpec, err := s.p.GetAllHosts(ctx)
		require.NoError(t, err)

		// Verify A actors are still at capacity (unchanged)
		var finalH1ACount, finalH2ACount int
		for _, activeActor := range finalSpec.ActiveActors {
			if activeActor.ActorType == "A" {
				switch activeActor.HostID {
				case SpecHostH1:
					finalH1ACount++
				case SpecHostH2:
					finalH2ACount++
				}
			}
		}

		assert.Equal(t, 3, finalH1ACount, "H1 should still have 3 active A actors")
		assert.Equal(t, 2, finalH2ACount, "H2 should still have 2 active A actors")

		// But we should have more total active actors due to the B actors we created
		assert.Greater(t, len(finalSpec.ActiveActors), len(spec.ActiveActors), "should have more total active actors after creating B actors")
	})

	t.Run("ActiveOnly - returns existing active actor", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Look up an existing actor that's already active on a healthy host
		// From spec, B-1 is active on H1 (healthy)
		ref := ref.ActorRef{ActorType: "B", ActorID: "B-1"}
		res, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{ActiveOnly: true})
		require.NoError(t, err)

		// Should return the existing host H1
		assert.Equal(t, SpecHostH1, res.HostID)
		assert.Equal(t, "127.0.0.1:4001", res.Address)
		assert.Equal(t, 5*time.Minute, res.IdleTimeout)
	})

	t.Run("ActiveOnly - returns ErrNoActor for inactive actor", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Try to look up an actor that doesn't exist/isn't active
		ref := ref.ActorRef{ActorType: "B", ActorID: "B-nonexistent"}
		_, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{ActiveOnly: true})
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrNoActor)
	})

	t.Run("ActiveOnly - returns ErrNoActor for actor on unhealthy host", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Look up an actor that exists only on unhealthy host H6
		// From GetSpec: D-1 is active on H6 (unhealthy)
		ref := ref.ActorRef{ActorType: "D", ActorID: "D-1"}
		_, err := s.p.LookupActor(ctx, ref, components.LookupActorOpts{ActiveOnly: true})
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrNoActor)
	})

	t.Run("ActiveOnly - returns ErrNoActor for actor on disallowed host", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Look up actor B-1 which is active on H1, but restrict to only H2
		// This should return ErrNoActor because the actor is on a disallowed host
		ref := ref.ActorRef{ActorType: "B", ActorID: "B-1"}
		opts := components.LookupActorOpts{
			ActiveOnly: true,
			Hosts:      []string{SpecHostH2},
		}
		_, err := s.p.LookupActor(ctx, ref, opts)
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrNoActor)
	})
}
