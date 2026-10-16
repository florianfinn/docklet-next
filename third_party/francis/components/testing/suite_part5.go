// Source: italypaleale/francis/components/testing/suite.go
// Revision: 23cd46831a543a73eec05ad034afd46245225876
// Retrieved: 2026-10-16
// License: MIT
// Deviations: Pinned provider test support restored for default tests; reserved example addresses; oversized suite split without behavior changes.

package comptesting

import (
	"bytes"
	"context"
	"fmt"
	"github.com/italypaleale/francis/components"
	"github.com/italypaleale/francis/internal/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"sync"
	"testing"
	"time"
)

func (s Suite) TestConcurrentDispatchJobs(t *testing.T) {
	const (
		jobHost = "0b000000-0000-4000-8000-0000000000c1"
		workers = 12
	)

	jobSeed := Spec{
		Hosts: HostSpecCollection{
			{HostID: jobHost, Address: "127.0.0.1:7200", LastHealthAgo: time.Second},
		},
		HostActorTypes: HostActorTypeSpecCollection{
			{HostID: jobHost, ActorType: "CONCURRENT_JOB", ActorIdleTimeout: 5 * time.Minute},
		},
	}

	t.Run("immediate same-key dispatch returns one lease", func(t *testing.T) {
		ctx := t.Context()
		err := s.p.Seed(ctx, jobSeed)
		require.NoError(t, err)

		// Release every caller together so the provider must serialize both insertion and lease acquisition
		jobRef := ref.NewAlarmRef("CONCURRENT_JOB", "immediate", "same-key")
		start := make(chan struct{})
		type dispatchResult struct {
			jobID string
			lease *ref.AlarmLease
			err   error
		}
		results := make(chan dispatchResult, workers)
		var wg sync.WaitGroup
		for range workers {
			wg.Go(func() {
				<-start
				jobID, _, lease, dispatchErr := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
					DueTime:        s.p.Now().Add(time.Second),
					Kind:           components.AlarmKindJob,
					JobMethod:      "process",
					LeaseImmediate: []string{jobHost},
				})
				results <- dispatchResult{jobID: jobID, lease: lease, err: dispatchErr}
			})
		}
		close(start)
		wg.Wait()
		close(results)

		// Every caller observes one durable identity and only the lease winner can enqueue it
		var firstID string
		var leased []*ref.AlarmLease
		for result := range results {
			require.NoError(t, result.err)
			require.NotEmpty(t, result.jobID)
			if firstID == "" {
				firstID = result.jobID
			}
			assert.Equal(t, firstID, result.jobID)
			if result.lease != nil {
				leased = append(leased, result.lease)
			}
		}
		require.Len(t, leased, 1)
		assert.Equal(t, firstID, leased[0].Key())
		_, err = s.p.GetLeasedAlarm(ctx, leased[0])
		require.NoError(t, err)

		jobs, err := s.p.ListJobs(ctx, jobRef.ActorType, jobRef.ActorID)
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		assert.Equal(t, firstID, jobs[0].JobID)
	})

	t.Run("future same-key dispatch always returns the stored ID", func(t *testing.T) {
		ctx := t.Context()
		err := s.p.Seed(ctx, jobSeed)
		require.NoError(t, err)

		// Future dispatches use the storage-only path but must retain the same concurrency contract
		jobRef := ref.NewAlarmRef("CONCURRENT_JOB", "future", "same-key")
		start := make(chan struct{})
		type dispatchResult struct {
			jobID string
			lease *ref.AlarmLease
			err   error
		}
		results := make(chan dispatchResult, workers)
		var wg sync.WaitGroup
		for range workers {
			wg.Go(func() {
				<-start
				jobID, _, lease, dispatchErr := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
					DueTime:   s.p.Now().Add(time.Hour),
					Kind:      components.AlarmKindJob,
					JobMethod: "process",
				})
				results <- dispatchResult{jobID: jobID, lease: lease, err: dispatchErr}
			})
		}
		close(start)
		wg.Wait()
		close(results)

		// A conflict must return the winner's ID even when its insert committed after another statement began
		var firstID string
		for result := range results {
			require.NoError(t, result.err)
			require.NotEmpty(t, result.jobID)
			assert.Nil(t, result.lease)
			if firstID == "" {
				firstID = result.jobID
			}
			assert.Equal(t, firstID, result.jobID)
		}

		jobs, err := s.p.ListJobs(ctx, jobRef.ActorType, jobRef.ActorID)
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		assert.Equal(t, firstID, jobs[0].JobID)
	})
}

func (s Suite) TestRemoveActor(t *testing.T) {
	t.Run("removes existing active actor", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Verify initial state - B-1 should be active on H1
		spec, err := s.p.GetAllHosts(ctx)
		require.NoError(t, err)

		// Find B-1 in active actors
		var foundActor *ActiveActorSpec
		for _, aa := range spec.ActiveActors {
			if aa.ActorType == "B" && aa.ActorID == "B-1" {
				foundActor = &aa
				break
			}
		}
		require.NotNil(t, foundActor, "B-1 should exist in initial test data")
		assert.Equal(t, SpecHostH1, foundActor.HostID)

		// Remove the actor
		ref := ref.ActorRef{ActorType: "B", ActorID: "B-1"}
		err = s.p.RemoveActor(ctx, ref)
		require.NoError(t, err)

		// Verify actor is no longer active
		spec, err = s.p.GetAllHosts(ctx)
		require.NoError(t, err)

		// B-1 should no longer be in active actors
		for _, aa := range spec.ActiveActors {
			if aa.ActorType == "B" && aa.ActorID == "B-1" {
				t.Fatalf("B-1 should have been removed but is still active on host %s", aa.HostID)
			}
		}
	})

	t.Run("returns ErrNoActor for non-existent actor", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Try to remove a non-existent actor
		ref := ref.ActorRef{ActorType: "B", ActorID: "NonExistent"}
		err := s.p.RemoveActor(ctx, ref)
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrNoActor)
	})

	t.Run("returns ErrNoActor for non-existent actor type", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Try to remove an actor with non-existent type
		ref := ref.ActorRef{ActorType: "NonExistentType", ActorID: "SomeID"}
		err := s.p.RemoveActor(ctx, ref)
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrNoActor)
	})

	t.Run("removes actor and frees up capacity", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// From GetSpec: Type A is at capacity (H1: 3/3, H2: 2/2)
		// First verify we can't create a new A actor
		_, err := s.p.LookupActor(ctx, ref.ActorRef{ActorType: "A", ActorID: "A-should-fail"}, components.LookupActorOpts{})
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrNoHost, "should fail when capacity is exhausted")

		// Remove one of the existing A actors (A-1 is on H1)
		aRef := ref.ActorRef{ActorType: "A", ActorID: "A-1"}
		err = s.p.RemoveActor(ctx, aRef)
		require.NoError(t, err)

		// Now we should be able to create a new A actor
		res, err := s.p.LookupActor(ctx, ref.ActorRef{ActorType: "A", ActorID: "A-new-after-removal"}, components.LookupActorOpts{})
		require.NoError(t, err)
		assert.NotEmpty(t, res.HostID)
		assert.Contains(t, []string{SpecHostH1, SpecHostH2}, res.HostID, "should be placed on one of the hosts that support A")

		// Verify the capacity was freed up correctly by checking final state
		spec, err := s.p.GetAllHosts(ctx)
		require.NoError(t, err)

		// Count A actors on each host
		var h1Count, h2Count int
		for _, aa := range spec.ActiveActors {
			if aa.ActorType == "A" {
				switch aa.HostID {
				case SpecHostH1:
					h1Count++
				case SpecHostH2:
					h2Count++
				}
			}
		}

		// Should have same total capacity (5) but with the new actor instead of A-1
		assert.Equal(t, 5, h1Count+h2Count, "should still have 5 A actors total")

		// Verify A-1 is gone and A-new-after-removal exists
		hasA1, hasNewA := false, false
		for _, aa := range spec.ActiveActors {
			if aa.ActorType != "A" {
				continue
			}

			if aa.ActorID == "A-1" {
				hasA1 = true
			}
			if aa.ActorID == "A-new-after-removal" {
				hasNewA = true
			}
		}
		assert.False(t, hasA1, "A-1 should be removed")
		assert.True(t, hasNewA, "A-new-after-removal should exist")
	})

	t.Run("removes multiple actors", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Get initial count
		spec, err := s.p.GetAllHosts(ctx)
		require.NoError(t, err)
		initialCount := len(spec.ActiveActors)

		// Remove multiple actors
		actors := []ref.ActorRef{
			{ActorType: "B", ActorID: "B-1"},
			{ActorType: "B", ActorID: "B-2"},
			{ActorType: "A", ActorID: "A-2"},
		}

		for _, ref := range actors {
			err = s.p.RemoveActor(ctx, ref)
			require.NoError(t, err, "should successfully remove actor %s", ref.String())
		}

		// Verify all actors were removed
		spec, err = s.p.GetAllHosts(ctx)
		require.NoError(t, err)

		assert.Len(t, spec.ActiveActors, initialCount-3, "should have 3 fewer active actors")

		// Verify none of the removed actors are still present
		for _, aa := range spec.ActiveActors {
			for _, ref := range actors {
				if aa.ActorType == ref.ActorType && aa.ActorID == ref.ActorID {
					t.Fatalf("Actor %s should have been removed but is still active", ref.String())
				}
			}
		}
	})

	t.Run("idempotent removal - removing same actor twice", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		ref := ref.ActorRef{ActorType: "B", ActorID: "B-1"}

		// Remove the actor first time - should succeed
		err := s.p.RemoveActor(ctx, ref)
		require.NoError(t, err)

		// Remove the same actor second time - should return ErrNoActor
		err = s.p.RemoveActor(ctx, ref)
		require.Error(t, err)
		require.ErrorIs(t, err, components.ErrNoActor)
	})

	t.Run("automatically cancels alarm leases when actor is removed", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Create a new actor and set an alarm for it
		aRef := ref.ActorRef{ActorType: "X", ActorID: "X-lease-test"}

		// First create the actor by looking it up (this activates it)
		lookupRes, err := s.p.LookupActor(ctx, aRef, components.LookupActorOpts{
			Hosts: []string{SpecHostH7, SpecHostH8},
		})
		require.NoError(t, err)
		assert.Contains(t, []string{SpecHostH7, SpecHostH8}, lookupRes.HostID)

		// Set an alarm for this actor
		alarmRef := ref.AlarmRef{
			ActorType: aRef.ActorType,
			ActorID:   aRef.ActorID,
			Name:      "test-alarm",
		}
		alarmReq := components.SetAlarmReq{
			// Overdue so it's fetched right away
			DueTime: s.p.Now().Add(-time.Second),
		}
		_, err = s.p.SetAlarm(ctx, alarmRef, alarmReq)
		require.NoError(t, err)

		// Fetch and lease the alarm
		fetchRes, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{lookupRes.HostID},
		})
		require.NoError(t, err)

		// Find our specific alarm lease
		var targetLease *ref.AlarmLease
		for _, lease := range fetchRes {
			alarmDetails, err := s.p.GetLeasedAlarm(ctx, lease)
			if err == nil && alarmDetails.ActorType == aRef.ActorType && alarmDetails.ActorID == aRef.ActorID {
				targetLease = lease
				break
			}
		}
		require.NotNil(t, targetLease, "should have found and leased the alarm for our test actor")

		// Verify the alarm is properly leased before removal
		_, err = s.p.GetLeasedAlarm(ctx, targetLease)
		require.NoError(t, err, "alarm should be properly leased before actor removal")

		// Remove the actor: this should automatically cancel any alarm leases via the database trigger
		err = s.p.RemoveActor(ctx, aRef)
		require.NoError(t, err)

		// Verify the alarm lease has been automatically canceled
		_, err = s.p.GetLeasedAlarm(ctx, targetLease)
		require.ErrorIs(t, err, components.ErrNoAlarm, "alarm lease should be automatically canceled after actor removal")

		// Verify the alarm still exists but is no longer leased by checking the database state
		spec, err := s.p.GetAllHosts(ctx)
		require.NoError(t, err)

		// Find the alarm in the database
		var foundAlarm *AlarmSpec
		for _, alarm := range spec.Alarms {
			if alarm.ActorType == aRef.ActorType && alarm.ActorID == aRef.ActorID && alarm.Name == "test-alarm" {
				foundAlarm = &alarm
				break
			}
		}
		require.NotNil(t, foundAlarm, "alarm should still exist in database")

		// But it should not have lease information anymore
		assert.Nil(t, foundAlarm.LeaseID, "alarm should not have lease ID after actor removal")
		assert.Nil(t, foundAlarm.LeaseExp, "alarm should not have lease expiration after actor removal")
	})
}

func (s Suite) TestState(t *testing.T) {
	expectCollection := func(t *testing.T, expected ActorStateSpecCollection) {
		t.Helper()
		rows, err := s.p.GetAllActorState(t.Context())
		require.NoError(t, err)
		assert.True(t, expected.Equal(rows), "unexpected actor state collection: got=%v expected=%v", rows, expected)
	}

	// Seed with empty database
	require.NoError(t, s.p.Seed(t.Context(), Spec{}))

	t.Run("get returns ErrNoState if no state", func(t *testing.T) {
		_, err := s.p.GetState(t.Context(), ref.ActorRef{ActorType: "TestType", ActorID: "actor-1"})
		require.ErrorIs(t, err, components.ErrNoState)
	})

	t.Run("delete returns ErrNoState if no state", func(t *testing.T) {
		err := s.p.DeleteState(t.Context(), ref.ActorRef{ActorType: "TestType", ActorID: "actor-1"})
		require.ErrorIs(t, err, components.ErrNoState)
	})

	t.Run("set get overwrite delete", func(t *testing.T) {
		ctx := t.Context()
		ref := ref.ActorRef{ActorType: "TestType", ActorID: "actor-1"}

		data1 := []byte("hello world")
		err := s.p.SetState(ctx, ref, data1, components.SetStateOpts{})
		require.NoError(t, err)

		got, err := s.p.GetState(ctx, ref)
		require.NoError(t, err)
		assert.True(t, bytes.Equal(data1, got))
		expectCollection(t, ActorStateSpecCollection{{ActorType: ref.ActorType, ActorID: ref.ActorID, Data: data1}})

		data2 := []byte("goodbye")
		err = s.p.SetState(ctx, ref, data2, components.SetStateOpts{})
		require.NoError(t, err)

		got, err = s.p.GetState(ctx, ref)
		require.NoError(t, err)
		assert.True(t, bytes.Equal(data2, got))
		expectCollection(t, ActorStateSpecCollection{{ActorType: ref.ActorType, ActorID: ref.ActorID, Data: data2}})

		err = s.p.SetState(ctx, ref, []byte{}, components.SetStateOpts{})
		require.NoError(t, err)

		got, err = s.p.GetState(ctx, ref)
		require.NoError(t, err)
		assert.Empty(t, got)
		expectCollection(t, ActorStateSpecCollection{{ActorType: ref.ActorType, ActorID: ref.ActorID, Data: []byte{}}})

		err = s.p.DeleteState(ctx, ref)
		require.NoError(t, err)

		_, err = s.p.GetState(ctx, ref)
		require.ErrorIs(t, err, components.ErrNoState)
		expectCollection(t, ActorStateSpecCollection{})

		err = s.p.DeleteState(ctx, ref)
		require.ErrorIs(t, err, components.ErrNoState)
	})

	t.Run("ttl expiration", func(t *testing.T) {
		ctx := t.Context()
		ref2 := ref.ActorRef{ActorType: "TestType", ActorID: "actor-ttl-1"}
		data := []byte("with-ttl")

		err := s.p.SetState(ctx, ref2, data, components.SetStateOpts{TTL: time.Second})
		require.NoError(t, err)

		_, err = s.p.GetState(ctx, ref2)
		require.NoError(t, err)
		expectCollection(t, ActorStateSpecCollection{{ActorType: ref2.ActorType, ActorID: ref2.ActorID, Data: data}})

		_ = s.p.AdvanceClock(1200 * time.Millisecond) //nolint:errcheck
		err = s.p.CleanupExpired(t.Context())
		require.NoError(t, err)

		_, err = s.p.GetState(ctx, ref2)
		require.ErrorIs(t, err, components.ErrNoState)
		expectCollection(t, ActorStateSpecCollection{})
	})

	t.Run("ttl extension on overwrite", func(t *testing.T) {
		ctx := t.Context()
		ref3 := ref.ActorRef{ActorType: "TestType", ActorID: "actor-ttl-extend"}
		data1 := []byte("first")
		data2 := []byte("second")

		err := s.p.SetState(ctx, ref3, data1, components.SetStateOpts{TTL: 2 * time.Second})
		require.NoError(t, err)
		expectCollection(t, ActorStateSpecCollection{{ActorType: ref3.ActorType, ActorID: ref3.ActorID, Data: data1}})

		_ = s.p.AdvanceClock(time.Second) //nolint:errcheck
		err = s.p.SetState(ctx, ref3, data2, components.SetStateOpts{TTL: 2 * time.Second})
		require.NoError(t, err)
		expectCollection(t, ActorStateSpecCollection{{ActorType: ref3.ActorType, ActorID: ref3.ActorID, Data: data2}})

		_ = s.p.AdvanceClock(1200 * time.Millisecond) //nolint:errcheck
		_, err = s.p.GetState(ctx, ref3)
		require.NoError(t, err)
		expectCollection(t, ActorStateSpecCollection{{ActorType: ref3.ActorType, ActorID: ref3.ActorID, Data: data2}})

		_ = s.p.AdvanceClock(1200 * time.Millisecond) //nolint:errcheck
		_, err = s.p.GetState(ctx, ref3)
		require.ErrorIs(t, err, components.ErrNoState)

		// GC hasn't run yet
		expectCollection(t, ActorStateSpecCollection{{ActorType: ref3.ActorType, ActorID: ref3.ActorID, Data: data2}})

		err = s.p.CleanupExpired(t.Context())
		require.NoError(t, err)
		expectCollection(t, ActorStateSpecCollection{})
	})

	t.Run("delete removes state with a live ttl", func(t *testing.T) {
		ctx := t.Context()
		ref4 := ref.ActorRef{ActorType: "TestType", ActorID: "actor-ttl-delete"}
		data := []byte("delete-me")

		// State with a TTL well in the future (i.e. not expired)
		err := s.p.SetState(ctx, ref4, data, components.SetStateOpts{TTL: time.Hour})
		require.NoError(t, err)
		expectCollection(t, ActorStateSpecCollection{{ActorType: ref4.ActorType, ActorID: ref4.ActorID, Data: data}})

		// Deleting live (non-expired) state must succeed and actually remove the row
		err = s.p.DeleteState(ctx, ref4)
		require.NoError(t, err)

		_, err = s.p.GetState(ctx, ref4)
		require.ErrorIs(t, err, components.ErrNoState)
		expectCollection(t, ActorStateSpecCollection{})

		// A second delete reports ErrNoState
		err = s.p.DeleteState(ctx, ref4)
		require.ErrorIs(t, err, components.ErrNoState)
	})
}

func (s Suite) TestListStates(t *testing.T) {
	// setState stores state for an actor of the given type
	setState := func(t *testing.T, ctx context.Context, actorType string, actorID string, data []byte, opts components.SetStateOpts) {
		t.Helper()
		err := s.p.SetState(ctx, ref.ActorRef{ActorType: actorType, ActorID: actorID}, data, opts)
		require.NoError(t, err)
	}

	// listIDs returns the actor IDs in a page, so tests can assert on the ordering without repeating the mapping
	listIDs := func(t *testing.T, ctx context.Context, req components.ListStatesReq) ([]string, bool) {
		t.Helper()
		res, err := s.p.ListStates(ctx, req)
		require.NoError(t, err)

		ids := make([]string, len(res.States))
		for i, state := range res.States {
			ids[i] = state.ActorID
		}
		return ids, res.HasMore
	}

	// Seed with empty database
	require.NoError(t, s.p.Seed(t.Context(), Spec{}))

	t.Run("returns an empty list when no state is stored", func(t *testing.T) {
		res, err := s.p.ListStates(t.Context(), components.ListStatesReq{ActorType: "ListEmpty"})
		require.NoError(t, err)
		assert.Empty(t, res.States)
		assert.False(t, res.HasMore)
	})

	t.Run("lists only the actors of the requested type", func(t *testing.T) {
		ctx := t.Context()

		setState(t, ctx, "ListTypeA", "actor-02", []byte("a2"), components.SetStateOpts{})
		setState(t, ctx, "ListTypeA", "actor-01", []byte("a1"), components.SetStateOpts{})
		setState(t, ctx, "ListTypeB", "actor-03", []byte("b3"), components.SetStateOpts{})

		// The result is ordered by actor ID, regardless of the order the states were written in
		ids, hasMore := listIDs(t, ctx, components.ListStatesReq{ActorType: "ListTypeA"})
		assert.Equal(t, []string{"actor-01", "actor-02"}, ids)
		assert.False(t, hasMore)

		ids, hasMore = listIDs(t, ctx, components.ListStatesReq{ActorType: "ListTypeB"})
		assert.Equal(t, []string{"actor-03"}, ids)
		assert.False(t, hasMore)
	})

	t.Run("omits the data unless it is requested", func(t *testing.T) {
		ctx := t.Context()

		res, err := s.p.ListStates(ctx, components.ListStatesReq{ActorType: "ListTypeA"})
		require.NoError(t, err)
		require.Len(t, res.States, 2)
		for _, state := range res.States {
			assert.Empty(t, state.Data, "data must not be returned for actor %s", state.ActorID)
		}
	})

	t.Run("returns the data when requested", func(t *testing.T) {
		ctx := t.Context()

		// State stored as an empty value is listed like any other, just with no data to return
		setState(t, ctx, "ListData", "actor-01", []byte("hello world"), components.SetStateOpts{})
		setState(t, ctx, "ListData", "actor-02", []byte{}, components.SetStateOpts{})

		res, err := s.p.ListStates(ctx, components.ListStatesReq{ActorType: "ListData", IncludeData: true})
		require.NoError(t, err)
		require.Len(t, res.States, 2)

		assert.Equal(t, "actor-01", res.States[0].ActorID)
		assert.True(t, bytes.Equal([]byte("hello world"), res.States[0].Data))
		assert.Equal(t, "actor-02", res.States[1].ActorID)
		assert.Empty(t, res.States[1].Data)
	})

	t.Run("excludes deleted state", func(t *testing.T) {
		ctx := t.Context()

		setState(t, ctx, "ListDeleted", "actor-01", []byte("gone"), components.SetStateOpts{})
		setState(t, ctx, "ListDeleted", "actor-02", []byte("stays"), components.SetStateOpts{})

		err := s.p.DeleteState(ctx, ref.ActorRef{ActorType: "ListDeleted", ActorID: "actor-01"})
		require.NoError(t, err)

		ids, _ := listIDs(t, ctx, components.ListStatesReq{ActorType: "ListDeleted"})
		assert.Equal(t, []string{"actor-02"}, ids)
	})

	t.Run("excludes expired state before it is garbage collected", func(t *testing.T) {
		ctx := t.Context()

		setState(t, ctx, "ListExpired", "actor-01", []byte("short"), components.SetStateOpts{TTL: time.Second})
		setState(t, ctx, "ListExpired", "actor-02", []byte("long"), components.SetStateOpts{TTL: time.Hour})
		setState(t, ctx, "ListExpired", "actor-03", []byte("forever"), components.SetStateOpts{})

		ids, _ := listIDs(t, ctx, components.ListStatesReq{ActorType: "ListExpired"})
		assert.Equal(t, []string{"actor-01", "actor-02", "actor-03"}, ids)

		// The expired row is still in the database at this point, so this asserts that listing filters on the expiration rather than relying on the cleanup
		_ = s.p.AdvanceClock(1200 * time.Millisecond) //nolint:errcheck
		ids, _ = listIDs(t, ctx, components.ListStatesReq{ActorType: "ListExpired"})
		assert.Equal(t, []string{"actor-02", "actor-03"}, ids)
	})

	t.Run("pages through the collection with after and limit", func(t *testing.T) {
		ctx := t.Context()

		const count = 5
		for i := 1; i <= count; i++ {
			setState(t, ctx, "ListPaged", fmt.Sprintf("actor-%02d", i), fmt.Appendf(nil, "data-%02d", i), components.SetStateOpts{})
		}

		// Walk the collection two at a time, using the last ID of each page as the cursor for the next one
		var (
			seen   []string
			cursor string
		)
		for range count {
			res, err := s.p.ListStates(ctx, components.ListStatesReq{ActorType: "ListPaged", After: cursor, Limit: 2, IncludeData: true})
			require.NoError(t, err)
			require.NotEmpty(t, res.States)
			require.LessOrEqual(t, len(res.States), 2)

			for _, state := range res.States {
				seen = append(seen, state.ActorID)
				// Paging must not affect the data, so each page carries the payload of the actors it contains
				assert.Equal(t, "data-"+strings.TrimPrefix(state.ActorID, "actor-"), string(state.Data))
			}

			cursor = res.States[len(res.States)-1].ActorID
			if !res.HasMore {
				break
			}
		}

		// Every actor is visited exactly once, in order, and the final page reports no more results
		assert.Equal(t, []string{"actor-01", "actor-02", "actor-03", "actor-04", "actor-05"}, seen)

		// A cursor past the end of the collection returns nothing
		ids, hasMore := listIDs(t, ctx, components.ListStatesReq{ActorType: "ListPaged", After: "actor-05"})
		assert.Empty(t, ids)
		assert.False(t, hasMore)

		// The cursor doesn't have to be an existing actor ID: listing resumes at the next ID after it
		ids, _ = listIDs(t, ctx, components.ListStatesReq{ActorType: "ListPaged", After: "actor-02a"})
		assert.Equal(t, []string{"actor-03", "actor-04", "actor-05"}, ids)
	})

	t.Run("limits the page to the default when no limit is requested", func(t *testing.T) {
		ctx := t.Context()

		// One state more than the default page size, so the first page is full and reports that more exist
		count := components.DefaultListStatesLimit + 1
		for i := 1; i <= count; i++ {
			setState(t, ctx, "ListDefaultLimit", fmt.Sprintf("actor-%04d", i), []byte("d"), components.SetStateOpts{})
		}

		res, err := s.p.ListStates(ctx, components.ListStatesReq{ActorType: "ListDefaultLimit"})
		require.NoError(t, err)
		assert.Len(t, res.States, components.DefaultListStatesLimit)
		assert.True(t, res.HasMore)

		// The last page holds the remainder and reports the end of the collection
		res, err = s.p.ListStates(ctx, components.ListStatesReq{ActorType: "ListDefaultLimit", After: res.States[len(res.States)-1].ActorID})
		require.NoError(t, err)
		assert.Len(t, res.States, count-components.DefaultListStatesLimit)
		assert.False(t, res.HasMore)
	})

	t.Run("caps a limit above the maximum", func(t *testing.T) {
		ctx := t.Context()

		// Asking for more than the provider allows is not an error: the request is served with the capped page size
		res, err := s.p.ListStates(ctx, components.ListStatesReq{ActorType: "ListDefaultLimit", Limit: components.MaxListStatesLimit + 1})
		require.NoError(t, err)
		assert.Len(t, res.States, components.DefaultListStatesLimit+1)
		assert.False(t, res.HasMore)
	})
}
