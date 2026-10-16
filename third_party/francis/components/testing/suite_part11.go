// Source: italypaleale/francis/components/testing/suite.go
// Revision: 23cd46831a543a73eec05ad034afd46245225876
// Retrieved: 2026-10-16
// License: MIT
// Deviations: Pinned provider test support restored for default tests; reserved example addresses; oversized suite split without behavior changes.

package comptesting

import (
	"context"
	"github.com/italypaleale/francis/components"
	"github.com/italypaleale/francis/internal/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// TestJobs exercises the job-specific provider methods: dispatch (with idempotency), get, list, cancel, dead-letter, and replay.
func (s Suite) TestJobs(t *testing.T) {
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

	t.Run("dispatch and get a live job", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		jobID := dispatch(t, ctx, "a1", "k1", "process", ref.AlarmProperties{DueTime: s.p.Now().Add(time.Hour)}, []byte("payload"))

		info, err := s.p.GetJob(ctx, jobID)
		require.NoError(t, err)
		assert.Equal(t, jobID, info.JobID)
		assert.Equal(t, "JOB", info.ActorType)
		assert.Equal(t, "a1", info.ActorID)
		assert.Equal(t, "process", info.Method)
		assert.Equal(t, components.JobStatusPending, info.Status)
		assert.False(t, info.CreatedAt.IsZero(), "created at should be derived from the UUIDv7 job ID")
	})

	t.Run("dispatch leases an upcoming new job", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		// Store a fully-populated job close enough to qualify for fetch-ahead scheduling
		jobRef := ref.NewAlarmRef("JOB", "preleased", "key")
		dueTime := s.p.Now().Add(time.Second)
		ttl := s.p.Now().Add(2 * time.Hour)
		jobID, _, lease, err := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
			DueTime:        dueTime,
			Interval:       "PT1H",
			TTL:            &ttl,
			Data:           []byte("payload"),
			Kind:           components.AlarmKindJob,
			JobMethod:      "process",
			LeaseImmediate: []string{jobHost},
		})
		require.NoError(t, err)
		require.NotNil(t, lease)
		assert.Equal(t, jobID, lease.Key())
		assert.Equal(t, jobRef, lease.AlarmRef())

		// Verify the returned lease authorizes the exact stored job row
		stored, err := s.p.GetLeasedAlarm(ctx, lease)
		require.NoError(t, err)
		assert.Equal(t, components.AlarmKindJob, stored.Kind)
		assert.WithinDuration(t, dueTime, stored.DueTime, time.Second)
		assert.Equal(t, "PT1H", stored.Interval)
		assert.Empty(t, stored.Cron)
		require.NotNil(t, stored.TTL)
		assert.WithinDuration(t, ttl, *stored.TTL, time.Second)
		assert.Equal(t, []byte("payload"), stored.Data)
		assert.Equal(t, "process", stored.JobMethod)
		placement, err := s.p.LookupActor(ctx, jobRef.ActorRef(), components.LookupActorOpts{
			ActiveOnly: true,
		})
		require.NoError(t, err)
		assert.Equal(t, jobHost, placement.HostID)

		// Re-dispatching the idempotency key preserves the first job and does not return its live lease twice
		duplicateID, _, duplicateLease, err := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
			DueTime:        dueTime.Add(time.Second),
			Kind:           components.AlarmKindJob,
			JobMethod:      "different",
			LeaseImmediate: []string{jobHost},
		})
		require.NoError(t, err)
		assert.Equal(t, jobID, duplicateID)
		assert.Nil(t, duplicateLease)
		_, err = s.p.GetLeasedAlarm(ctx, lease)
		require.NoError(t, err)
	})

	t.Run("re-dispatch leases an existing unleased job", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		// Create the idempotent job without offering a host for immediate scheduling
		jobRef := ref.NewAlarmRef("JOB", "existing-unleased", "key")
		dueTime := s.p.Now().Add(time.Second)
		jobID, _, initialLease, err := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
			DueTime:   dueTime,
			Kind:      components.AlarmKindJob,
			JobMethod: "original",
			Data:      []byte("original"),
		})
		require.NoError(t, err)
		require.Nil(t, initialLease)

		// Re-dispatching can acquire its lease but must retain the first dispatch's properties
		duplicateID, _, lease, err := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
			DueTime:        dueTime.Add(time.Second),
			Kind:           components.AlarmKindJob,
			JobMethod:      "replacement",
			Data:           []byte("replacement"),
			LeaseImmediate: []string{jobHost},
		})
		require.NoError(t, err)
		require.NotNil(t, lease)
		assert.Equal(t, jobID, duplicateID)
		assert.Equal(t, jobID, lease.Key())

		stored, err := s.p.GetLeasedAlarm(ctx, lease)
		require.NoError(t, err)
		assert.WithinDuration(t, dueTime, stored.DueTime, time.Second)
		assert.Equal(t, "original", stored.JobMethod)
		assert.Equal(t, []byte("original"), stored.Data)
	})

	t.Run("re-dispatch uses the storage-only path for an incoming future schedule", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		// Store an upcoming occurrence without offering an immediate host
		jobRef := ref.NewAlarmRef("JOB", "incoming-future", "key")
		dueTime := s.p.Now().Add(time.Second)
		jobID, _, initialLease, err := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
			DueTime:   dueTime,
			Kind:      components.AlarmKindJob,
			JobMethod: "original",
			Data:      []byte("original"),
		})
		require.NoError(t, err)
		require.Nil(t, initialLease)

		// A duplicate request outside fetch-ahead avoids the transactional lease path even though the stored row is earlier
		duplicateID, _, lease, err := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
			DueTime:        s.p.Now().Add(time.Hour),
			Kind:           components.AlarmKindJob,
			JobMethod:      "replacement",
			Data:           []byte("replacement"),
			LeaseImmediate: []string{jobHost},
		})
		require.NoError(t, err)
		assert.Equal(t, jobID, duplicateID)
		assert.Nil(t, lease)

		info, err := s.p.GetJob(ctx, jobID)
		require.NoError(t, err)
		assert.WithinDuration(t, dueTime, info.DueTime, time.Second)
		assert.Equal(t, "original", info.Method)
		assert.Equal(t, components.JobStatusPending, info.Status)
		_, err = s.p.LookupActor(ctx, jobRef.ActorRef(), components.LookupActorOpts{ActiveOnly: true})
		require.ErrorIs(t, err, components.ErrNoActor)
	})

	t.Run("re-dispatch keeps an existing future job unleased", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		// The first dispatch establishes a future schedule that an idempotency conflict cannot replace
		jobRef := ref.NewAlarmRef("JOB", "existing-future", "key")
		dueTime := s.p.Now().Add(time.Hour)
		jobID, _, initialLease, err := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
			DueTime:   dueTime,
			Kind:      components.AlarmKindJob,
			JobMethod: "original",
		})
		require.NoError(t, err)
		require.Nil(t, initialLease)

		// An immediate duplicate request must not lease or place the retained future occurrence
		duplicateID, _, lease, err := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
			DueTime:        s.p.Now(),
			Kind:           components.AlarmKindJob,
			JobMethod:      "replacement",
			LeaseImmediate: []string{jobHost},
		})
		require.NoError(t, err)
		assert.Equal(t, jobID, duplicateID)
		assert.Nil(t, lease)

		info, err := s.p.GetJob(ctx, jobID)
		require.NoError(t, err)
		assert.WithinDuration(t, dueTime, info.DueTime, time.Second)
		assert.Equal(t, "original", info.Method)
		assert.Equal(t, components.JobStatusPending, info.Status)
		_, err = s.p.LookupActor(ctx, jobRef.ActorRef(), components.LookupActorOpts{ActiveOnly: true})
		require.ErrorIs(t, err, components.ErrNoActor)
	})

	t.Run("dispatch leases a job whose actor is active on an allowed host", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		// Place the actor on the host that will be offered for immediate execution
		jobRef := ref.NewAlarmRef("JOB", "active-allowed", "key")
		placement, err := s.p.LookupActor(ctx, jobRef.ActorRef(), components.LookupActorOpts{Hosts: []string{jobHost}})
		require.NoError(t, err)
		require.Equal(t, jobHost, placement.HostID)

		// Dispatching reuses the placement while acquiring the job lease
		jobID, _, lease, err := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
			DueTime:        s.p.Now().Add(time.Second),
			Kind:           components.AlarmKindJob,
			JobMethod:      "process",
			LeaseImmediate: []string{jobHost},
		})
		require.NoError(t, err)
		require.NotNil(t, lease)
		assert.Equal(t, jobID, lease.Key())

		placement, err = s.p.LookupActor(ctx, jobRef.ActorRef(), components.LookupActorOpts{ActiveOnly: true})
		require.NoError(t, err)
		assert.Equal(t, jobHost, placement.HostID)
	})

	t.Run("dispatch leaves an upcoming job durable without an eligible host", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, Spec{
			Hosts: HostSpecCollection{
				{HostID: jobHost, Address: "127.0.0.1:7100", LastHealthAgo: time.Second},
			},
			HostActorTypes: HostActorTypeSpecCollection{
				{HostID: jobHost, ActorType: "OTHER", ActorIdleTimeout: 5 * time.Minute},
			},
		}))

		// The offered host cannot execute this actor type, so only the durable job is created
		jobRef := ref.NewAlarmRef("JOB", "no-host", "key")
		jobID, _, lease, err := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
			DueTime:        s.p.Now().Add(time.Second),
			Kind:           components.AlarmKindJob,
			JobMethod:      "process",
			LeaseImmediate: []string{jobHost},
		})
		require.NoError(t, err)
		assert.Nil(t, lease)

		info, err := s.p.GetJob(ctx, jobID)
		require.NoError(t, err)
		assert.Equal(t, components.JobStatusPending, info.Status)
		_, err = s.p.LookupActor(ctx, jobRef.ActorRef(), components.LookupActorOpts{ActiveOnly: true})
		require.ErrorIs(t, err, components.ErrNoActor)
	})

	t.Run("dispatch does not move an actor from a healthy disallowed host", func(t *testing.T) {
		const secondJobHost = "0b000000-0000-4000-8000-0000000000b2"

		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, Spec{
			Hosts: HostSpecCollection{
				{HostID: jobHost, Address: "127.0.0.1:7100", LastHealthAgo: time.Second},
				{HostID: secondJobHost, Address: "127.0.0.1:7101", LastHealthAgo: time.Second},
			},
			HostActorTypes: HostActorTypeSpecCollection{
				{HostID: jobHost, ActorType: "JOB", ActorIdleTimeout: 5 * time.Minute},
				{HostID: secondJobHost, ActorType: "JOB", ActorIdleTimeout: 5 * time.Minute},
			},
		}))

		// Pin the actor to the first host before offering only the second host for the job lease
		jobRef := ref.NewAlarmRef("JOB", "active-disallowed", "key")
		placement, err := s.p.LookupActor(ctx, jobRef.ActorRef(), components.LookupActorOpts{Hosts: []string{jobHost}})
		require.NoError(t, err)
		require.Equal(t, jobHost, placement.HostID)

		jobID, _, lease, err := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
			DueTime:        s.p.Now().Add(time.Second),
			Kind:           components.AlarmKindJob,
			JobMethod:      "process",
			LeaseImmediate: []string{secondJobHost},
		})
		require.NoError(t, err)
		assert.Nil(t, lease)
		_, err = s.p.GetJob(ctx, jobID)
		require.NoError(t, err)

		placement, err = s.p.LookupActor(ctx, jobRef.ActorRef(), components.LookupActorOpts{ActiveOnly: true})
		require.NoError(t, err)
		assert.Equal(t, jobHost, placement.HostID)
	})

	t.Run("re-dispatch replaces an expired job lease", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		// Acquire the first lease while the original host is healthy
		jobRef := ref.NewAlarmRef("JOB", "expired-lease", "key")
		jobID, _, originalLease, err := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
			DueTime:        s.p.Now().Add(time.Second),
			Kind:           components.AlarmKindJob,
			JobMethod:      "process",
			LeaseImmediate: []string{jobHost},
		})
		require.NoError(t, err)
		require.NotNil(t, originalLease)

		// Advance past both lease expiry and host health, then add a fresh eligible host
		err = s.p.AdvanceClock(2 * time.Minute)
		require.NoError(t, err)
		hostRes, err := s.p.RegisterHost(ctx, components.RegisterHostReq{
			Address: "127.0.0.1:7102",
			ActorTypes: []components.ActorHostType{{
				ActorType:   "JOB",
				IdleTimeout: 5 * time.Minute,
			}},
		})
		require.NoError(t, err)

		// The same occurrence gets a fresh lease rather than a new durable identity
		duplicateID, _, lease, err := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
			DueTime:        s.p.Now(),
			Kind:           components.AlarmKindJob,
			JobMethod:      "replacement",
			LeaseImmediate: []string{hostRes.HostID},
		})
		require.NoError(t, err)
		require.NotNil(t, lease)
		assert.Equal(t, jobID, duplicateID)
		assert.Equal(t, jobID, lease.Key())
		assert.NotEqual(t, originalLease.LeaseID(), lease.LeaseID())

		_, err = s.p.GetLeasedAlarm(ctx, originalLease)
		require.ErrorIs(t, err, components.ErrNoAlarm)
		_, err = s.p.GetLeasedAlarm(ctx, lease)
		require.NoError(t, err)
	})

	t.Run("dispatch leaves a future job unleased", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		jobRef := ref.NewAlarmRef("JOB", "future", "key")
		dueTime := s.p.Now().Add(time.Hour)
		jobID, _, lease, err := s.p.DispatchJob(ctx, jobRef, components.SetAlarmReq{
			DueTime:        dueTime,
			Kind:           components.AlarmKindJob,
			JobMethod:      "process",
			LeaseImmediate: []string{jobHost},
		})
		require.NoError(t, err)
		assert.NotEmpty(t, jobID)
		assert.Nil(t, lease)
		info, err := s.p.GetJob(ctx, jobID)
		require.NoError(t, err)
		assert.Equal(t, components.JobStatusPending, info.Status)
		assert.WithinDuration(t, dueTime, info.DueTime, time.Second)
		_, err = s.p.LookupActor(ctx, jobRef.ActorRef(), components.LookupActorOpts{ActiveOnly: true})
		require.ErrorIs(t, err, components.ErrNoActor)
	})

	t.Run("idempotency key dedups re-dispatch", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		// Same actor + same key yields one job
		id1 := dispatch(t, ctx, "a1", "key", "process", ref.AlarmProperties{DueTime: s.p.Now().Add(time.Hour)}, nil)
		id2 := dispatch(t, ctx, "a1", "key", "process", ref.AlarmProperties{DueTime: s.p.Now().Add(time.Hour)}, nil)
		assert.Equal(t, id1, id2, "re-dispatching with the same key must return the same job ID")

		// A different key yields a distinct job
		id3 := dispatch(t, ctx, "a1", "other", "process", ref.AlarmProperties{DueTime: s.p.Now().Add(time.Hour)}, nil)
		assert.NotEqual(t, id1, id3)
	})

	t.Run("list jobs returns live jobs for the actor", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		id1 := dispatch(t, ctx, "list-actor", "j1", "process", ref.AlarmProperties{DueTime: s.p.Now().Add(time.Hour)}, nil)
		id2 := dispatch(t, ctx, "list-actor", "j2", "process", ref.AlarmProperties{DueTime: s.p.Now().Add(time.Hour)}, nil)

		jobs, err := s.p.ListJobs(ctx, "JOB", "list-actor")
		require.NoError(t, err)
		ids := make([]string, len(jobs))
		for i, j := range jobs {
			ids[i] = j.JobID
		}
		assert.ElementsMatch(t, []string{id1, id2}, ids)
	})

	t.Run("delete removes a live job", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		jobID := dispatch(t, ctx, "cancel-actor", "c1", "process", ref.AlarmProperties{DueTime: s.p.Now().Add(time.Hour)}, nil)

		err := s.p.DeleteJob(ctx, "JOB", "cancel-actor", jobID, components.DeleteJobReq{})
		require.NoError(t, err)

		_, err = s.p.GetJob(ctx, jobID)
		require.ErrorIs(t, err, components.ErrNoJob)

		// Deleting again reports the job is gone
		err = s.p.DeleteJob(ctx, "JOB", "cancel-actor", jobID, components.DeleteJobReq{})
		require.ErrorIs(t, err, components.ErrNoJob)
	})

	t.Run("delete is scoped to the actor that owns the job", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		jobID := dispatch(t, ctx, "scope-actor", "s1", "process", ref.AlarmProperties{DueTime: s.p.Now().Add(time.Hour)}, nil)

		// Another actor's ID must not reach this job
		err := s.p.DeleteJob(ctx, "JOB", "other-actor", jobID, components.DeleteJobReq{})
		require.ErrorIs(t, err, components.ErrNoJob)

		_, err = s.p.GetJob(ctx, jobID)
		require.NoError(t, err, "the job must survive a deletion scoped to a different actor")
	})

	t.Run("one delete verb removes a job that has ended", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		// The caller uses the same verb whether the job is still scheduled or has already ended
		jobID := dispatch(t, ctx, "unified-actor", "u1", "process", ref.AlarmProperties{DueTime: s.p.Now()}, nil)
		lease := leaseFor(t, ctx, jobID)
		err := s.p.DeadLetterAlarm(ctx, lease, components.DeadLetterAlarmReq{Reason: "boom", Attempts: 3})
		require.NoError(t, err)

		err = s.p.DeleteJob(ctx, "JOB", "unified-actor", jobID, components.DeleteJobReq{})
		require.NoError(t, err)

		_, err = s.p.GetJob(ctx, jobID)
		require.ErrorIs(t, err, components.ErrNoJob)
	})

	t.Run("a completed job is retained when the dispatch asked for it", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, jobSeed()))

		jobID := dispatch(t, ctx, "done-actor", "dn1", "process", ref.AlarmProperties{DueTime: s.p.Now()}, []byte("done-payload"))
		lease := leaseFor(t, ctx, jobID)

		err := s.p.CompleteJob(ctx, lease, components.CompleteJobReq{Attempts: 1, Retention: time.Hour})
		require.NoError(t, err)

		// The record reports the run as completed rather than failed, and carries no error
		info, err := s.p.GetJob(ctx, jobID)
		require.NoError(t, err)
		assert.Equal(t, components.JobStatusCompleted, info.Status)
		assert.Equal(t, 1, info.Attempts)
		assert.Empty(t, info.LastError)
		assert.False(t, info.EndedAt.IsZero(), "a terminal job records when it ended")

		// A completed record drops the input, which is only ever needed to replay a dead job
		done, err := s.p.GetTerminalJob(ctx, jobID)
		require.NoError(t, err)
		assert.Equal(t, components.JobStatusCompleted, done.Status)
		assert.Empty(t, done.Data, "a completed record should not carry the job payload")
		require.NotNil(t, done.Expiration)

		// It appears in the actor's listing alongside any live job
		jobs, err := s.p.ListJobs(ctx, "JOB", "done-actor")
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		assert.Equal(t, components.JobStatusCompleted, jobs[0].Status)

		// The live alarm row is gone, so the lease no longer resolves
		_, err = s.p.GetLeasedAlarm(ctx, lease)
		require.ErrorIs(t, err, components.ErrNoAlarm)

		// A completed job has nothing to retry
		_, err = s.p.RetryDeadJob(ctx, jobID)
		require.ErrorIs(t, err, components.ErrNoJob)

		// The same delete verb removes it
		err = s.p.DeleteJob(ctx, "JOB", "done-actor", jobID, components.DeleteJobReq{})
		require.NoError(t, err)
		_, err = s.p.GetJob(ctx, jobID)
		require.ErrorIs(t, err, components.ErrNoJob)
	})

	s.testRemainingJobs(t)
}
