// Source: italypaleale/francis/components/testing/suite.go
// Revision: 23cd46831a543a73eec05ad034afd46245225876
// Retrieved: 2026-10-16
// License: MIT
// Deviations: Pinned provider test support restored for default tests; reserved example addresses; oversized suite split without behavior changes.

package comptesting

import (
	"context"
	"errors"
	"fmt"
	"github.com/italypaleale/francis/components"
	"github.com/italypaleale/francis/internal/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// TestJobs exercises the job-specific provider methods: dispatch (with idempotency), get, list, cancel, dead-letter, and replay.
func (s Suite) testRemainingJobs(t *testing.T) {
	const jobHost = "0b000000-0000-4000-8000-0000000000b1"

	// jobSeed returns a minimal spec with a single healthy host supporting the "JOB" actor type
	jobSeed := func() Spec {
		return Spec{
			Hosts: HostSpecCollection{
				{HostID: jobHost, Address: "127.0.0.1:7100", LastHealthAgo: time.Second},
			},
			HostActorTypes: HostActorTypeSpecCollection{
				{HostID: jobHost, ActorType: "JOB", ActorIdleTimeout: 5 * time.Minute, ActorConcurrencyLimit: 0},
			},
		}
	}

	// dispatch creates a job and returns its ID
	dispatch := func(t *testing.T, ctx context.Context, actorID string, name string, method string, props ref.AlarmProperties, data []byte) string {
		t.Helper()

		props.Data = data
		jobID, _, _, err := s.p.DispatchJob(ctx, ref.NewAlarmRef("JOB", actorID, name), components.SetAlarmReq{
			AlarmProperties: props,
			Kind:            components.AlarmKindJob,
			JobMethod:       method,
		})
		require.NoError(t, err)
		require.NotEmpty(t, jobID)
		return jobID
	}

	// leaseFor dispatches the actor's alarms via the fetcher and returns the lease whose ID matches jobID
	leaseFor := func(t *testing.T, ctx context.Context, jobID string) *ref.AlarmLease {
		t.Helper()

		leases, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{jobHost},
		})
		require.NoError(t, err)

		for _, l := range leases {
			if l.Key() == jobID {
				return l
			}
		}

		t.Fatalf("did not lease the dispatched job %q", jobID)
		return nil
	}

	t.Run("a terminal record reads as gone once its retention has elapsed", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		jobID := dispatch(t, ctx, "expire-actor", "e1", "process", ref.AlarmProperties{DueTime: s.p.Now()}, nil)
		lease := leaseFor(t, ctx, jobID)

		err := s.p.CompleteJob(ctx, lease, components.CompleteJobReq{Attempts: 1, Retention: time.Second})
		require.NoError(t, err)

		// It is readable while the retention holds
		_, err = s.p.GetJob(ctx, jobID)
		require.NoError(t, err)

		// Past the retention it reads as gone, whether or not the collector has run yet
		_ = s.p.AdvanceClock(2 * time.Second) //nolint:errcheck

		_, err = s.p.GetJob(ctx, jobID)
		require.ErrorIs(t, err, components.ErrNoJob)

		_, err = s.p.GetTerminalJob(ctx, jobID)
		require.ErrorIs(t, err, components.ErrNoJob)

		jobs, err := s.p.ListJobs(ctx, "JOB", "expire-actor")
		require.NoError(t, err)
		assert.Empty(t, jobs)

		// The collector then removes it for good
		err = s.p.CleanupExpired(ctx)
		require.NoError(t, err)
		_, err = s.p.GetJob(ctx, jobID)
		require.ErrorIs(t, err, components.ErrNoJob)
	})

	t.Run("dead-letter a one-shot job", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		due := s.p.Now()
		jobID := dispatch(t, ctx, "dead-actor", "d1", "process", ref.AlarmProperties{DueTime: due}, []byte("dead-payload"))
		lease := leaseFor(t, ctx, jobID)

		err := s.p.DeadLetterAlarm(ctx, lease, components.DeadLetterAlarmReq{Reason: "boom", Attempts: 3})
		require.NoError(t, err)

		// GetJob now reports the dead-lettered job, with attempts and the last error
		info, err := s.p.GetJob(ctx, jobID)
		require.NoError(t, err)
		assert.Equal(t, components.JobStatusDeadLettered, info.Status)
		assert.Equal(t, 3, info.Attempts)
		assert.Equal(t, "boom", info.LastError)

		// The dead job carries its input so it can be replayed
		dead, err := s.p.GetTerminalJob(ctx, jobID)
		require.NoError(t, err)
		assert.Equal(t, jobID, dead.JobID)
		assert.Equal(t, "JOB", dead.ActorType)
		assert.Equal(t, "dead-actor", dead.ActorID)
		assert.Equal(t, "process", dead.Method)
		assert.Equal(t, []byte("dead-payload"), dead.Data)
		assert.Equal(t, components.JobStatusDeadLettered, dead.Status)
		assert.Equal(t, 3, dead.Attempts)
		assert.Equal(t, "boom", dead.LastError)
		assert.False(t, dead.EndedAt.IsZero(), "a terminal record says when the job ended")

		// The occurrence's own due time is kept, which is what tells an operator when the run that failed was meant to happen
		assert.WithinDuration(t, due, dead.OriginalDue, time.Millisecond)

		// The live alarm row is gone, so the lease no longer resolves
		_, err = s.p.GetLeasedAlarm(ctx, lease)
		require.ErrorIs(t, err, components.ErrNoAlarm)

		// Deleting the dead job removes it entirely
		err = s.p.DeleteJob(ctx, "JOB", "dead-actor", jobID, components.DeleteJobReq{})
		require.NoError(t, err)
		_, err = s.p.GetJob(ctx, jobID)
		require.ErrorIs(t, err, components.ErrNoJob)
	})

	t.Run("retry re-dispatches a dead job atomically", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		jobID := dispatch(t, ctx, "retry-actor", "rt1", "process", ref.AlarmProperties{DueTime: s.p.Now()}, []byte("payload"))
		lease := leaseFor(t, ctx, jobID)
		require.NoError(t, s.p.DeadLetterAlarm(ctx, lease, components.DeadLetterAlarmReq{Reason: "boom", Attempts: 3}))

		// Replay it: a new live job appears and the dead-letter record is gone, both atomically
		newID, err := s.p.RetryDeadJob(ctx, jobID)
		require.NoError(t, err)
		assert.NotEqual(t, jobID, newID, "replay should mint a new job ID")

		// The original ID no longer resolves: neither a live job nor a dead one remains under it
		_, err = s.p.GetJob(ctx, jobID)
		require.ErrorIs(t, err, components.ErrNoJob)

		// The new job is live and carries the original method and data
		info, err := s.p.GetJob(ctx, newID)
		require.NoError(t, err)
		assert.NotEqual(t, components.JobStatusDeadLettered, info.Status)
		assert.Equal(t, "process", info.Method)

		// Retrying a job that is no longer dead-lettered reports it as missing
		_, err = s.p.RetryDeadJob(ctx, jobID)
		require.ErrorIs(t, err, components.ErrNoJob)
	})

	// A caller needs to tell starting work from finding it already under way, and the insert is the only place that can say so: a status read before it would race the job that is being created
	t.Run("dispatch stores an initial state atomically with the job", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		labels := &components.WorkflowLabels{Status: "pending", Version: 2, Parent: "p1"}
		listLabeled := func(t *testing.T, filter components.WorkflowLabels) []string {
			t.Helper()
			res, err := s.p.ListStates(ctx, components.ListStatesReq{ActorType: "JOB", WorkflowLabels: &filter})
			require.NoError(t, err)
			ids := make([]string, len(res.States))
			for i, state := range res.States {
				ids[i] = state.ActorID
			}
			return ids
		}

		// A new job stores the state and its labels, on both the storage-only and the leasing paths
		for _, actorID := range []string{"initial-future", "initial-immediate"} {
			req := components.SetAlarmReq{
				DueTime:      s.p.Now().Add(time.Hour),
				Kind:         components.AlarmKindJob,
				JobMethod:    "start",
				InitialState: &components.InitialState{Data: []byte("placeholder"), WorkflowLabels: labels},
			}
			if actorID == "initial-immediate" {
				req.DueTime = s.p.Now()
				req.LeaseImmediate = []string{jobHost}
			}
			_, created, _, err := s.p.DispatchJob(ctx, ref.NewAlarmRef("JOB", actorID, "start"), req)
			require.NoError(t, err)
			assert.True(t, created)

			data, err := s.p.GetState(ctx, ref.ActorRef{ActorType: "JOB", ActorID: actorID})
			require.NoError(t, err)
			assert.Equal(t, []byte("placeholder"), data)
		}
		assert.Equal(t, []string{"initial-future", "initial-immediate"}, listLabeled(t, components.WorkflowLabels{Parent: "p1"}))
		assert.Equal(t, []string{"initial-future", "initial-immediate"}, listLabeled(t, components.WorkflowLabels{Status: "pending", Version: 2}))

		// Existing live state is never replaced, whether the dispatch creates a job or coalesces onto one
		existingRef := ref.ActorRef{ActorType: "JOB", ActorID: "initial-existing"}
		require.NoError(t, s.p.SetState(ctx, existingRef, []byte("journal"), components.SetStateOpts{WorkflowLabels: &components.WorkflowLabels{Status: "running", Version: 2}}))
		for range 2 {
			_, _, _, err := s.p.DispatchJob(ctx, ref.NewAlarmRef("JOB", "initial-existing", "start"), components.SetAlarmReq{
				DueTime:      s.p.Now().Add(time.Hour),
				Kind:         components.AlarmKindJob,
				JobMethod:    "start",
				InitialState: &components.InitialState{Data: []byte("placeholder"), WorkflowLabels: labels},
			})
			require.NoError(t, err)
		}
		data, err := s.p.GetState(ctx, existingRef)
		require.NoError(t, err)
		assert.Equal(t, []byte("journal"), data)
		assert.Equal(t, []string{"initial-existing"}, listLabeled(t, components.WorkflowLabels{Status: "running", Version: 2}))

		// A dispatch that coalesces onto a live job still stores the state when the actor has none
		for _, immediate := range []bool{false, true} {
			actorID := "initial-coalesced"
			if immediate {
				actorID = "initial-coalesced-immediate"
			}
			jobRef := ref.NewAlarmRef("JOB", actorID, "start")
			req := components.SetAlarmReq{
				DueTime:   s.p.Now().Add(time.Hour),
				Kind:      components.AlarmKindJob,
				JobMethod: "start",
			}
			if immediate {
				req.DueTime = s.p.Now()
				req.LeaseImmediate = []string{jobHost}
			}
			jobID, _, _, err := s.p.DispatchJob(ctx, jobRef, req)
			require.NoError(t, err)

			req.InitialState = &components.InitialState{Data: []byte("late"), WorkflowLabels: labels}
			duplicateID, created, _, err := s.p.DispatchJob(ctx, jobRef, req)
			require.NoError(t, err)
			assert.Equal(t, jobID, duplicateID)
			assert.False(t, created)

			data, err = s.p.GetState(ctx, jobRef.ActorRef())
			require.NoError(t, err)
			assert.Equal(t, []byte("late"), data)
		}

		// Expired state is treated as absent, so the initial state replaces it
		expiredRef := ref.ActorRef{ActorType: "JOB", ActorID: "initial-expired"}
		require.NoError(t, s.p.SetState(ctx, expiredRef, []byte("old"), components.SetStateOpts{TTL: time.Second}))
		require.NoError(t, s.p.AdvanceClock(2*time.Second))
		_, _, _, err = s.p.DispatchJob(ctx, ref.NewAlarmRef("JOB", "initial-expired", "start"), components.SetAlarmReq{
			DueTime:      s.p.Now().Add(time.Hour),
			Kind:         components.AlarmKindJob,
			JobMethod:    "start",
			InitialState: &components.InitialState{Data: []byte("placeholder"), WorkflowLabels: labels},
		})
		require.NoError(t, err)
		data, err = s.p.GetState(ctx, expiredRef)
		require.NoError(t, err)
		assert.Equal(t, []byte("placeholder"), data)

		// The stored state does not expire on its own
		require.NoError(t, s.p.AdvanceClock(time.Hour))
		data, err = s.p.GetState(ctx, expiredRef)
		require.NoError(t, err)
		assert.Equal(t, []byte("placeholder"), data)
	})

	t.Run("dispatch reports whether it created the job", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		jobRef := ref.NewAlarmRef("JOB", "created-flag", "key")
		req := components.SetAlarmReq{
			AlarmProperties: ref.AlarmProperties{DueTime: s.p.Now().Add(time.Hour)},
			Kind:            components.AlarmKindJob,
			JobMethod:       "process",
		}

		jobID, created, _, err := s.p.DispatchJob(ctx, jobRef, req)
		require.NoError(t, err)
		require.NotEmpty(t, jobID)
		assert.True(t, created, "the first dispatch under an idempotency key creates the job")

		// A second dispatch coalesces onto the job that already holds the key, and gets its ID rather than a new one
		duplicateID, created, _, err := s.p.DispatchJob(ctx, jobRef, req)
		require.NoError(t, err)
		assert.Equal(t, jobID, duplicateID)
		assert.False(t, created)

		// A job due inside fetch-ahead takes the leasing path, which has to answer the same way
		immediateRef := ref.NewAlarmRef("JOB", "created-flag", "immediate")
		immediateReq := components.SetAlarmReq{
			AlarmProperties: ref.AlarmProperties{DueTime: s.p.Now()},
			Kind:            components.AlarmKindJob,
			JobMethod:       "process",
			LeaseImmediate:  []string{jobHost},
		}

		immediateID, created, _, err := s.p.DispatchJob(ctx, immediateRef, immediateReq)
		require.NoError(t, err)
		require.NotEmpty(t, immediateID)
		assert.True(t, created)

		duplicateID, created, _, err = s.p.DispatchJob(ctx, immediateRef, immediateReq)
		require.NoError(t, err)
		assert.Equal(t, immediateID, duplicateID)
		assert.False(t, created)
	})

	// A cancellation only ever removes work that has not run, so the record a finished job left behind has to survive it
	// The scope belongs inside the deletion rather than in a status check before it, since a job that finalizes in between would otherwise have its record destroyed
	t.Run("a live-only delete leaves a terminal record alone", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		// A job that has not run yet is what a cancellation is for
		pending := dispatch(t, ctx, "live-only", "p1", "process", ref.AlarmProperties{DueTime: s.p.Now().Add(time.Hour)}, nil)
		err := s.p.DeleteJob(ctx, "JOB", "live-only", pending, components.DeleteJobReq{LiveOnly: true})
		require.NoError(t, err)
		_, err = s.p.GetJob(ctx, pending)
		require.ErrorIs(t, err, components.ErrNoJob)

		// A job that has already completed is not the cancellation's to remove
		done := dispatch(t, ctx, "live-only", "p2", "process", ref.AlarmProperties{DueTime: s.p.Now()}, nil)
		lease := leaseFor(t, ctx, done)
		require.NoError(t, s.p.CompleteJob(ctx, lease, components.CompleteJobReq{Attempts: 1, Retention: time.Hour}))

		err = s.p.DeleteJob(ctx, "JOB", "live-only", done, components.DeleteJobReq{LiveOnly: true})
		require.ErrorIs(t, err, components.ErrNoJob)

		info, err := s.p.GetJob(ctx, done)
		require.NoError(t, err, "the completed record must survive a cancellation")
		assert.Equal(t, components.JobStatusCompleted, info.Status)

		// Without the scope the same call removes the record, which is what an operator asking to delete a job means
		err = s.p.DeleteJob(ctx, "JOB", "live-only", done, components.DeleteJobReq{})
		require.NoError(t, err)
		_, err = s.p.GetJob(ctx, done)
		require.ErrorIs(t, err, components.ErrNoJob)
	})

	// A record past its retention is already invisible to reads, so replaying it would re-dispatch work a caller can no longer see
	t.Run("retry treats a dead job past its retention as missing", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		jobID := dispatch(t, ctx, "expired-actor", "ex1", "process", ref.AlarmProperties{DueTime: s.p.Now()}, []byte("payload"))
		lease := leaseFor(t, ctx, jobID)
		require.NoError(t, s.p.DeadLetterAlarm(ctx, lease, components.DeadLetterAlarmReq{Reason: "boom", Attempts: 3, Retention: time.Minute}))

		// Cleanup runs on its own interval, so a record can outlive its retention in the table while reads already hide it
		_ = s.p.AdvanceClock(2 * time.Minute) //nolint:errcheck

		_, err := s.p.GetJob(ctx, jobID)
		require.ErrorIs(t, err, components.ErrNoJob)

		_, err = s.p.RetryDeadJob(ctx, jobID)
		require.ErrorIs(t, err, components.ErrNoJob)
	})

	t.Run("dead-letter a repeating occurrence keeps the recurrence", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		jobID := dispatch(t, ctx, "repeat-actor", "r1", "process", ref.AlarmProperties{DueTime: s.p.Now(), Interval: "PT1H"}, nil)
		lease := leaseFor(t, ctx, jobID)

		next := s.p.Now().Add(time.Hour)
		err := s.p.DeadLetterAlarm(ctx, lease, components.DeadLetterAlarmReq{Reason: "boom", Attempts: 1, Reschedule: true, NextDueTime: next})
		require.NoError(t, err)

		// The job ID still resolves to the recurrence, due at its next occurrence: a schedule keeps one identity for its whole life, so a caller that holds the ID can still reconcile or delete it
		info, err := s.p.GetJob(ctx, jobID)
		require.NoError(t, err)
		assert.NotEqual(t, components.JobStatusDeadLettered, info.Status)
		assert.WithinDuration(t, next, info.DueTime, time.Millisecond)

		// The failed occurrence is recorded alongside it, under an ID of its own
		jobs, err := s.p.ListJobs(ctx, "JOB", "repeat-actor")
		require.NoError(t, err)
		var (
			live, dead   int
			occurrenceID string
		)
		for _, j := range jobs {
			switch j.Status {
			case components.JobStatusDeadLettered:
				dead++
				occurrenceID = j.JobID
				assert.NotEqual(t, jobID, j.JobID, "the ended occurrence must have an ID of its own")
			default:
				live++
				assert.Equal(t, jobID, j.JobID, "the recurrence must keep the job ID")
			}
		}
		assert.Equal(t, 1, live, "the recurrence should still have one live job")
		assert.Equal(t, 1, dead, "the failed occurrence should be dead-lettered")

		// The record keeps the schedule the occurrence belonged to, so a reader can tell a recurrence's occurrence from a one-shot after the fact
		require.NotEmpty(t, occurrenceID)
		occurrence, err := s.p.GetTerminalJob(ctx, occurrenceID)
		require.NoError(t, err)
		assert.Equal(t, "PT1H", occurrence.Interval)
		assert.Empty(t, occurrence.Cron, "an interval schedule carries no cron expression")
	})

	t.Run("a completed repeating occurrence is recorded and the recurrence continues", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		// The cadence is short so the next occurrence can be leased back without moving the clock past the host's health check deadline
		jobID := dispatch(t, ctx, "done-repeat-actor", "dr1", "process", ref.AlarmProperties{DueTime: s.p.Now(), Interval: "PT1S"}, []byte("payload"))
		lease := leaseFor(t, ctx, jobID)

		next := s.p.Now().Add(time.Second)
		err := s.p.CompleteJob(ctx, lease, components.CompleteJobReq{Attempts: 1, Retention: time.Hour, Reschedule: true, NextDueTime: next})
		require.NoError(t, err)

		// The job ID still resolves to the recurrence, due at its next occurrence
		info, err := s.p.GetJob(ctx, jobID)
		require.NoError(t, err)
		assert.False(t, info.Status.IsTerminal())
		assert.WithinDuration(t, next, info.DueTime, time.Millisecond)

		// The occurrence that ran is recorded under an ID of its own, so each occurrence leaves its own record without stopping the schedule or changing its identity
		jobs, err := s.p.ListJobs(ctx, "JOB", "done-repeat-actor")
		require.NoError(t, err)
		var live, completed int
		for _, j := range jobs {
			if j.Status.IsTerminal() {
				completed++
				assert.NotEqual(t, jobID, j.JobID, "the ended occurrence must have an ID of its own")
				continue
			}
			live++
			assert.Equal(t, jobID, j.JobID, "the recurrence must keep the job ID")
		}
		assert.Equal(t, 1, live, "the recurrence should still have one live job")
		assert.Equal(t, 1, completed, "the occurrence that ran should be recorded")

		// The next occurrence still has the input every occurrence is delivered with
		// Only the record of the occurrence that ended drops the payload, never the row it was read out of
		_ = s.p.AdvanceClock(time.Second) //nolint:errcheck

		nextLease := leaseFor(t, ctx, jobID)
		alarm, err := s.p.GetLeasedAlarm(ctx, nextLease)
		require.NoError(t, err)
		assert.Equal(t, "process", alarm.JobMethod)
		assert.Equal(t, []byte("payload"), alarm.Data)
		assert.Equal(t, "PT1S", alarm.Interval)
	})

	// A worker halts itself once it has reported, and deactivating an actor releases the leases of its alarms
	// The occurrence being finalized must survive that, or it is left behind to be leased and delivered again on the next poll, forever
	t.Run("a job can be finalized after its own actor released the lease", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			finalize func(t *testing.T, ctx context.Context, lease *ref.AlarmLease)
		}{
			{
				name: "completed",
				finalize: func(t *testing.T, ctx context.Context, lease *ref.AlarmLease) {
					err := s.p.CompleteJob(ctx, lease, components.CompleteJobReq{
						Attempts:  1,
						Retention: time.Hour,
					})
					require.NoError(t, err)
				},
			},
			{
				name: "dead-lettered",
				finalize: func(t *testing.T, ctx context.Context, lease *ref.AlarmLease) {
					err := s.p.DeadLetterAlarm(ctx, lease, components.DeadLetterAlarmReq{
						Reason:   "boom",
						Attempts: 3,
					})
					require.NoError(t, err)
				},
			},
			{
				name: "deleted",
				finalize: func(t *testing.T, ctx context.Context, lease *ref.AlarmLease) {
					err := s.p.DeleteLeasedAlarm(ctx, lease)
					require.NoError(t, err)
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				ctx := t.Context()
				require.NoError(t, s.p.Seed(ctx, jobSeed()))

				actorID := "halt-" + tc.name
				jobID := dispatch(t, ctx, actorID, "h1", "process", ref.AlarmProperties{DueTime: s.p.Now()}, nil)
				lease := leaseFor(t, ctx, jobID)

				// The actor halting itself from its own handler is what releases the lease
				err := s.p.RemoveActor(ctx, ref.NewActorRef("JOB", actorID))
				require.NoError(t, err)

				tc.finalize(t, ctx, lease)

				// Whatever the outcome, the occurrence must no longer be live, or the next poll runs it again
				info, err := s.p.GetJob(ctx, jobID)
				if errors.Is(err, components.ErrNoJob) {
					return
				}

				require.NoError(t, err)
				assert.True(t, info.Status.IsTerminal(), "a finalized occurrence must not be live, got %q", info.Status)
			})
		}
	})

	// A repeating job keeps one ID for the life of its schedule, so the alarm ID alone cannot tell one occurrence from the next
	// An execution whose lease was released must therefore not finalize the occurrence that replaced it: that would record work that never ran and reschedule the recurrence from a stale time
	t.Run("a released lease cannot finalize a later occurrence of a recurring job", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			finalize func(ctx context.Context, lease *ref.AlarmLease) error
		}{
			{
				name: "CompleteJob",
				finalize: func(ctx context.Context, lease *ref.AlarmLease) error {
					return s.p.CompleteJob(ctx, lease, components.CompleteJobReq{
						Attempts:    1,
						Retention:   time.Hour,
						Reschedule:  true,
						NextDueTime: s.p.Now().Add(2 * time.Hour),
					})
				},
			},
			{
				name: "DeadLetterAlarm",
				finalize: func(ctx context.Context, lease *ref.AlarmLease) error {
					return s.p.DeadLetterAlarm(ctx, lease, components.DeadLetterAlarmReq{
						Reason:      "boom",
						Attempts:    3,
						Reschedule:  true,
						NextDueTime: s.p.Now().Add(2 * time.Hour),
					})
				},
			},
			{
				name: "DeleteLeasedAlarm",
				finalize: func(ctx context.Context, lease *ref.AlarmLease) error {
					return s.p.DeleteLeasedAlarm(ctx, lease)
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				ctx := t.Context()
				require.NoError(t, s.p.Seed(ctx, jobSeed()))

				actorID := "stale-" + tc.name
				jobID := dispatch(t, ctx, actorID, "recurring", "process", ref.AlarmProperties{DueTime: s.p.Now(), Interval: "PT1H"}, []byte("payload"))
				stale := leaseFor(t, ctx, jobID)

				// Another host takes the occurrence this execution is still running and completes it, which records the occurrence and moves the recurrence on
				err := s.p.CompleteJob(ctx, stale, components.CompleteJobReq{
					Attempts:    1,
					Retention:   time.Hour,
					Reschedule:  true,
					NextDueTime: s.p.Now().Add(time.Hour),
				})
				require.NoError(t, err)

				// The stale execution's own finalization now names an occurrence that has already been accounted for
				err = tc.finalize(ctx, stale)
				require.ErrorIs(t, err, components.ErrNoAlarm)

				// The recurrence is still live and still due where the other host left it
				info, err := s.p.GetJob(ctx, jobID)
				require.NoError(t, err)
				assert.Equal(t, components.JobStatusPending, info.Status, "the occurrence that replaced it must survive")
				assert.WithinDuration(t, s.p.Now().Add(time.Hour), info.DueTime, time.Second, "the stale execution must not reschedule the recurrence")
			})
		}
	})

	// The lease predicate the three finalizers share accepts a lease the job's own actor released, which is the case above
	// This pins what it must still reject, because a finalizer that ended an occurrence on a lease it does not hold would end work another host is running
	t.Run("a finalizer rejects a lease it does not hold", func(t *testing.T) {
		finalizers := []struct {
			name     string
			finalize func(ctx context.Context, lease *ref.AlarmLease) error
		}{
			{
				name: "CompleteJob",
				finalize: func(ctx context.Context, lease *ref.AlarmLease) error {
					return s.p.CompleteJob(ctx, lease, components.CompleteJobReq{Attempts: 1, Retention: time.Hour})
				},
			},
			{
				name: "DeadLetterAlarm",
				finalize: func(ctx context.Context, lease *ref.AlarmLease) error {
					return s.p.DeadLetterAlarm(ctx, lease, components.DeadLetterAlarmReq{Reason: "boom", Attempts: 3})
				},
			},
			{
				name: "DeleteLeasedAlarm",
				finalize: func(ctx context.Context, lease *ref.AlarmLease) error {
					return s.p.DeleteLeasedAlarm(ctx, lease)
				},
			},
		}

		cases := []struct {
			name string
			// lease returns the lease to attempt the finalization with, given the one the row actually holds
			lease func(t *testing.T, ctx context.Context, held *ref.AlarmLease) *ref.AlarmLease
			// stillLive says whether the occurrence should survive the rejected attempt, which it does unless the case removed it
			stillLive bool
		}{
			{
				name: "a lease ID another host holds",
				lease: func(t *testing.T, ctx context.Context, held *ref.AlarmLease) *ref.AlarmLease {
					return ref.NewAlarmLease(held.AlarmRef(), held.Key(), held.DueTime(), "e731a719-0c1c-4c41-9c92-a44e0e8ef681")
				},
				stillLive: true,
			},
			{
				name: "a lease that has expired",
				lease: func(t *testing.T, ctx context.Context, held *ref.AlarmLease) *ref.AlarmLease {
					// Past the lease duration the occurrence is another host's to take, so this execution may no longer end it
					_ = s.p.AdvanceClock(2 * time.Minute) //nolint:errcheck
					return held
				},
				stillLive: true,
			},
			{
				name: "an alarm that is already gone",
				lease: func(t *testing.T, ctx context.Context, held *ref.AlarmLease) *ref.AlarmLease {
					err := s.p.DeleteLeasedAlarm(ctx, held)
					require.NoError(t, err)
					return held
				},
				stillLive: false,
			},
		}

		for _, f := range finalizers {
			t.Run(f.name, func(t *testing.T) {
				for i, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						ctx := t.Context()
						require.NoError(t, s.p.Seed(ctx, jobSeed()))

						actorID := fmt.Sprintf("reject-%s-%d", f.name, i)
						jobID := dispatch(t, ctx, actorID, "r1", "process", ref.AlarmProperties{DueTime: s.p.Now()}, []byte("payload"))
						held := leaseFor(t, ctx, jobID)

						err := f.finalize(ctx, tc.lease(t, ctx, held))
						require.ErrorIs(t, err, components.ErrNoAlarm)

						info, err := s.p.GetJob(ctx, jobID)
						if !tc.stillLive {
							require.ErrorIs(t, err, components.ErrNoJob)
							return
						}

						// The rejected attempt must leave the occurrence exactly as it was: still live, and with nothing recorded as having ended
						require.NoError(t, err)
						assert.False(t, info.Status.IsTerminal(), "a rejected finalization must leave the occurrence live, got %q", info.Status)

						_, err = s.p.GetTerminalJob(ctx, jobID)
						require.ErrorIs(t, err, components.ErrNoJob, "a rejected finalization must not record a terminal job")
					})
				}
			})
		}
	})

	t.Run("get and delete report missing jobs", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		_, err := s.p.GetJob(ctx, "11111111-1111-7111-8111-111111111111")
		require.ErrorIs(t, err, components.ErrNoJob)

		_, err = s.p.GetTerminalJob(ctx, "11111111-1111-7111-8111-111111111111")
		require.ErrorIs(t, err, components.ErrNoJob)

		err = s.p.DeleteJob(ctx, "JOB", "missing-actor", "11111111-1111-7111-8111-111111111111", components.DeleteJobReq{})
		require.ErrorIs(t, err, components.ErrNoJob)
	})
}
