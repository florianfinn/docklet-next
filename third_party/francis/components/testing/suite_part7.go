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

func (s Suite) TestSetAndLeaseAlarm(t *testing.T) {
	registerHost := func(t *testing.T, actorType string) string {
		t.Helper()

		res, err := s.p.RegisterHost(t.Context(), components.RegisterHostReq{
			Address:            "203.0.113.4:8080",
			ExistingHostID:     "",
			JoinToken:          "",
			JoinTokenExpiresAt: time.Time{},
			ActorTypes: []components.ActorHostType{{
				ActorType:           actorType,
				IdleTimeout:         time.Minute,
				ConcurrencyLimit:    0,
				DeactivationTimeout: 0,
				MaxAttempts:         0,
				InitialRetryDelay:   0,
			}},
		})
		require.NoError(t, err)
		return res.HostID
	}
	getAlarmSpec := func(t *testing.T, alarmRef ref.AlarmRef) AlarmSpec {
		t.Helper()

		spec, err := s.p.GetAllHosts(t.Context())
		require.NoError(t, err)
		for i := range spec.Alarms {
			alarm := spec.Alarms[i]
			if alarm.ActorType == alarmRef.ActorType && alarm.ActorID == alarmRef.ActorID && alarm.Name == alarmRef.Name {
				return alarm
			}
		}

		t.Fatalf("alarm %s was not found", alarmRef)
		return AlarmSpec{}
	}

	t.Run("leases an upcoming alarm whose actor is not active", func(t *testing.T) {
		// Start with one healthy host that can execute the alarm
		err := s.p.Seed(t.Context(), Spec{
			Hosts:          nil,
			HostActorTypes: nil,
			ActiveActors:   nil,
			Alarms:         nil,
		})
		require.NoError(t, err)
		hostID := registerHost(t, "LeaseActor")

		// Store an alarm close enough to qualify for fetch-ahead scheduling
		alarmRef := ref.NewAlarmRef("LeaseActor", "actor-1", "wake")
		lease, err := s.p.SetAlarm(t.Context(), alarmRef, components.SetAlarmReq{
			DueTime:        s.p.Now().Add(time.Second),
			Kind:           components.AlarmKindAlarm,
			LeaseImmediate: []string{hostID},
		})
		require.NoError(t, err)

		// Verify the lease returned by the atomic store authorizes access
		require.NotNil(t, lease)
		assert.Equal(t, alarmRef, lease.AlarmRef())

		stored, err := s.p.GetLeasedAlarm(t.Context(), lease)
		require.NoError(t, err)
		assert.Equal(t, alarmRef, stored.AlarmRef)
		placement, err := s.p.LookupActor(t.Context(), alarmRef.ActorRef(), components.LookupActorOpts{ActiveOnly: true})
		require.NoError(t, err)
		assert.Equal(t, hostID, placement.HostID)

		// Repeating the same request must preserve the live lease without returning it for duplicate enqueueing
		duplicateLease, err := s.p.SetAlarm(t.Context(), alarmRef, components.SetAlarmReq{
			DueTime:        lease.DueTime(),
			LeaseImmediate: []string{hostID},
		})
		require.NoError(t, err)
		assert.Nil(t, duplicateLease)
		_, err = s.p.GetLeasedAlarm(t.Context(), lease)
		require.NoError(t, err)
	})

	t.Run("leases an upcoming alarm whose actor is active on an allowed host", func(t *testing.T) {
		// Place the actor before storing its alarm
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)
		hostID := registerHost(t, "LeaseActor")
		actorRef := ref.NewActorRef("LeaseActor", "actor-active")
		placement, err := s.p.LookupActor(t.Context(), actorRef, components.LookupActorOpts{Hosts: []string{hostID}})
		require.NoError(t, err)
		require.Equal(t, hostID, placement.HostID)

		// Reuse the allowed placement while acquiring the alarm lease
		alarmRef := ref.NewAlarmRef(actorRef.ActorType, actorRef.ActorID, "wake")
		lease, err := s.p.SetAlarm(t.Context(), alarmRef, components.SetAlarmReq{
			DueTime:        s.p.Now().Add(time.Second),
			LeaseImmediate: []string{hostID},
		})
		require.NoError(t, err)
		require.NotNil(t, lease)

		stored, err := s.p.GetLeasedAlarm(t.Context(), lease)
		require.NoError(t, err)
		assert.Equal(t, alarmRef, stored.AlarmRef)
		placement, err = s.p.LookupActor(t.Context(), actorRef, components.LookupActorOpts{ActiveOnly: true})
		require.NoError(t, err)
		assert.Equal(t, hostID, placement.HostID)
	})

	t.Run("replaces an unleased alarm and leases the replacement", func(t *testing.T) {
		// Store the original alarm without requesting a lease
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)
		hostID := registerHost(t, "LeaseActor")
		alarmRef := ref.NewAlarmRef("LeaseActor", "replace-unleased", "wake")
		initialLease, err := s.p.SetAlarm(t.Context(), alarmRef, components.SetAlarmReq{
			DueTime: s.p.Now().Add(2 * time.Second),
			Data:    []byte("original"),
		})
		require.NoError(t, err)
		require.Nil(t, initialLease)
		original := getAlarmSpec(t, alarmRef)
		require.Nil(t, original.LeaseID)

		// Replace it inside fetch-ahead and acquire a lease for the new row
		updatedDueTime := s.p.Now().Add(3 * time.Second)
		replacementLease, err := s.p.SetAlarm(t.Context(), alarmRef, components.SetAlarmReq{
			DueTime:        updatedDueTime,
			Data:           []byte("replacement"),
			LeaseImmediate: []string{hostID},
		})
		require.NoError(t, err)
		require.NotNil(t, replacementLease)

		replacement := getAlarmSpec(t, alarmRef)
		assert.NotEqual(t, original.AlarmID, replacement.AlarmID)
		assert.Equal(t, replacementLease.Key(), replacement.AlarmID)
		require.NotNil(t, replacement.LeaseID)
		stored, err := s.p.GetLeasedAlarm(t.Context(), replacementLease)
		require.NoError(t, err)
		assert.WithinDuration(t, updatedDueTime, stored.DueTime, time.Second)
		assert.Equal(t, []byte("replacement"), stored.Data)
	})

	t.Run("replaces a leased alarm and leases the upcoming replacement", func(t *testing.T) {
		// Lease the original alarm and record its durable identity
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)
		hostID := registerHost(t, "LeaseActor")
		alarmRef := ref.NewAlarmRef("LeaseActor", "replace-leased-upcoming", "wake")
		originalLease, err := s.p.SetAlarm(t.Context(), alarmRef, components.SetAlarmReq{
			DueTime:        s.p.Now().Add(2 * time.Second),
			Data:           []byte("original"),
			LeaseImmediate: []string{hostID},
		})
		require.NoError(t, err)
		require.NotNil(t, originalLease)

		// Replacing it inside fetch-ahead invalidates the old lease and returns a lease for the new row
		updatedDueTime := s.p.Now().Add(4 * time.Second)
		replacementLease, err := s.p.SetAlarm(t.Context(), alarmRef, components.SetAlarmReq{
			DueTime:        updatedDueTime,
			Data:           []byte("replacement"),
			LeaseImmediate: []string{hostID},
		})
		require.NoError(t, err)
		require.NotNil(t, replacementLease)
		assert.NotEqual(t, originalLease.Key(), replacementLease.Key())

		_, err = s.p.GetLeasedAlarm(t.Context(), originalLease)
		require.ErrorIs(t, err, components.ErrNoAlarm)
		stored, err := s.p.GetLeasedAlarm(t.Context(), replacementLease)
		require.NoError(t, err)
		assert.WithinDuration(t, updatedDueTime, stored.DueTime, time.Second)
		assert.Equal(t, []byte("replacement"), stored.Data)
	})

	t.Run("replaces a leased alarm without leasing a replacement outside fetch-ahead", func(t *testing.T) {
		// Lease the original alarm while its due time is inside fetch-ahead
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)
		hostID := registerHost(t, "LeaseActor")
		alarmRef := ref.NewAlarmRef("LeaseActor", "replace-leased-future", "wake")
		originalLease, err := s.p.SetAlarm(t.Context(), alarmRef, components.SetAlarmReq{
			DueTime:        s.p.Now().Add(2 * time.Second),
			Data:           []byte("original"),
			LeaseImmediate: []string{hostID},
		})
		require.NoError(t, err)
		require.NotNil(t, originalLease)

		// Moving it outside fetch-ahead uses the fast path and leaves the new row unleased
		updatedDueTime := s.p.Now().Add(2 * time.Minute)
		replacementLease, err := s.p.SetAlarm(t.Context(), alarmRef, components.SetAlarmReq{
			DueTime:        updatedDueTime,
			Data:           []byte("replacement"),
			LeaseImmediate: []string{hostID},
		})
		require.NoError(t, err)
		require.Nil(t, replacementLease)

		_, err = s.p.GetLeasedAlarm(t.Context(), originalLease)
		require.ErrorIs(t, err, components.ErrNoAlarm)
		replacement := getAlarmSpec(t, alarmRef)
		assert.NotEqual(t, originalLease.Key(), replacement.AlarmID)
		assert.Nil(t, replacement.LeaseID)
		stored, err := s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)
		assert.WithinDuration(t, updatedDueTime, stored.DueTime, time.Second)
		assert.Equal(t, []byte("replacement"), stored.Data)
	})

	t.Run("leaves a future alarm unleased", func(t *testing.T) {
		// A capable host alone is insufficient when the alarm is outside the fetch-ahead horizon
		err := s.p.Seed(t.Context(), Spec{
			Hosts:          nil,
			HostActorTypes: nil,
			ActiveActors:   nil,
			Alarms:         nil,
		})
		require.NoError(t, err)
		hostID := registerHost(t, "LeaseActor")
		alarmRef := ref.NewAlarmRef("LeaseActor", "actor-2", "wake")
		lease, err := s.p.SetAlarm(t.Context(), alarmRef, components.SetAlarmReq{
			DueTime:        s.p.Now().Add(24 * time.Hour),
			Interval:       "",
			Cron:           "",
			TTL:            nil,
			Data:           nil,
			Kind:           components.AlarmKindAlarm,
			JobMethod:      "",
			LeaseImmediate: []string{hostID},
		})
		require.NoError(t, err)
		assert.Nil(t, lease)
		_, err = s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)
		_, err = s.p.LookupActor(t.Context(), alarmRef.ActorRef(), components.LookupActorOpts{ActiveOnly: true})
		require.ErrorIs(t, err, components.ErrNoActor)
	})

	t.Run("leaves an upcoming alarm unleased without an eligible host", func(t *testing.T) {
		// The allowed host does not advertise the alarm's actor type
		err := s.p.Seed(t.Context(), Spec{
			Hosts:          nil,
			HostActorTypes: nil,
			ActiveActors:   nil,
			Alarms:         nil,
		})
		require.NoError(t, err)
		hostID := registerHost(t, "OtherActor")
		alarmRef := ref.NewAlarmRef("LeaseActor", "actor-3", "wake")
		lease, err := s.p.SetAlarm(t.Context(), alarmRef, components.SetAlarmReq{
			DueTime:        s.p.Now().Add(time.Second),
			Kind:           components.AlarmKindAlarm,
			LeaseImmediate: []string{hostID},
		})
		require.NoError(t, err)
		assert.Nil(t, lease)
		_, err = s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)
	})

	t.Run("does not move an actor from a healthy disallowed host", func(t *testing.T) {
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)
		actorType := components.ActorHostType{ActorType: "LeaseActor", IdleTimeout: time.Minute}
		firstHost, err := s.p.RegisterHost(t.Context(), components.RegisterHostReq{
			Address:    "203.0.113.4:8080",
			ActorTypes: []components.ActorHostType{actorType},
		})
		require.NoError(t, err)
		secondHost, err := s.p.RegisterHost(t.Context(), components.RegisterHostReq{
			Address:    "203.0.113.5:8080",
			ActorTypes: []components.ActorHostType{actorType},
		})
		require.NoError(t, err)

		// Pin the actor to the first runtime before asking the second runtime to store and lease its alarm
		actorRef := ref.NewActorRef("LeaseActor", "actor-4")
		placement, err := s.p.LookupActor(t.Context(), actorRef, components.LookupActorOpts{Hosts: []string{firstHost.HostID}})
		require.NoError(t, err)
		require.Equal(t, firstHost.HostID, placement.HostID)

		alarmRef := ref.NewAlarmRef(actorRef.ActorType, actorRef.ActorID, "wake")
		lease, err := s.p.SetAlarm(t.Context(), alarmRef, components.SetAlarmReq{
			DueTime:        s.p.Now().Add(time.Second),
			LeaseImmediate: []string{secondHost.HostID},
		})
		require.NoError(t, err)
		assert.Nil(t, lease)

		// The alarm is durable but the existing placement remains owned by the first runtime
		_, err = s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)
		placement, err = s.p.LookupActor(t.Context(), actorRef, components.LookupActorOpts{ActiveOnly: true})
		require.NoError(t, err)
		assert.Equal(t, firstHost.HostID, placement.HostID)
	})
}

func (s Suite) TestDeleteAlarm(t *testing.T) {
	t.Run("delete existing alarm", func(t *testing.T) {
		// Seed with test data
		err := s.p.Seed(t.Context(), GetSpec())
		require.NoError(t, err)

		// Delete an existing alarm from the test spec
		alarmRef := ref.AlarmRef{
			ActorType: "A",
			ActorID:   "A-1",
			Name:      "Alarm-A-1",
		}

		// Verify alarm exists first
		_, err = s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)

		// Delete the alarm
		err = s.p.DeleteAlarm(t.Context(), alarmRef)
		require.NoError(t, err)

		// Verify alarm no longer exists
		_, err = s.p.GetAlarm(t.Context(), alarmRef)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})

	t.Run("returns ErrNoAlarm for non-existent alarm", func(t *testing.T) {
		// Seed with empty database
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)

		// Try to delete a non-existent alarm
		alarmRef := ref.AlarmRef{
			ActorType: "NonExistentActor",
			ActorID:   "non-existent-id",
			Name:      "non-existent-alarm",
		}

		err = s.p.DeleteAlarm(t.Context(), alarmRef)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})

	t.Run("delete one alarm does not affect others", func(t *testing.T) {
		// Seed with empty database
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)

		// Create multiple alarms for the same actor
		actorType := "TestActor"
		actorID := "test-id"
		now := time.Now()

		alarm1Ref := ref.AlarmRef{ActorType: actorType, ActorID: actorID, Name: "alarm1"}
		alarm2Ref := ref.AlarmRef{ActorType: actorType, ActorID: actorID, Name: "alarm2"}
		alarm3Ref := ref.AlarmRef{ActorType: actorType, ActorID: actorID, Name: "alarm3"}

		for i, alarmRef := range []ref.AlarmRef{alarm1Ref, alarm2Ref, alarm3Ref} {
			setReq := components.SetAlarmReq{
				DueTime: now.Add(time.Duration(i+1) * time.Hour),
				Data:    fmt.Appendf(nil, `{"alarm": %d}`, i+1),
			}

			_, err = s.p.SetAlarm(t.Context(), alarmRef, setReq)
			require.NoError(t, err)
		}

		// Verify all alarms exist
		for _, alarmRef := range []ref.AlarmRef{alarm1Ref, alarm2Ref, alarm3Ref} {
			_, err = s.p.GetAlarm(t.Context(), alarmRef)
			require.NoError(t, err)
		}

		// Delete the middle alarm
		err = s.p.DeleteAlarm(t.Context(), alarm2Ref)
		require.NoError(t, err)

		// Verify alarm2 no longer exists
		_, err = s.p.GetAlarm(t.Context(), alarm2Ref)
		require.ErrorIs(t, err, components.ErrNoAlarm)

		// Verify other alarms still exist
		_, err = s.p.GetAlarm(t.Context(), alarm1Ref)
		require.NoError(t, err)
		_, err = s.p.GetAlarm(t.Context(), alarm3Ref)
		require.NoError(t, err)
	})

	t.Run("delete alarm with different actor types", func(t *testing.T) {
		// Seed with empty database
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)

		// Create alarms with same actor ID but different types
		now := time.Now()
		actorID := "shared-id"
		alarmName := "shared-name"

		alarm1Ref := ref.AlarmRef{ActorType: "TypeA", ActorID: actorID, Name: alarmName}
		alarm2Ref := ref.AlarmRef{ActorType: "TypeB", ActorID: actorID, Name: alarmName}

		for _, alarmRef := range []ref.AlarmRef{alarm1Ref, alarm2Ref} {
			setReq := components.SetAlarmReq{
				DueTime: now.Add(1 * time.Hour),
				Data:    fmt.Appendf(nil, `{"type": "%s"}`, alarmRef.ActorType),
			}

			_, err = s.p.SetAlarm(t.Context(), alarmRef, setReq)
			require.NoError(t, err)
		}

		// Delete alarm for TypeA
		err = s.p.DeleteAlarm(t.Context(), alarm1Ref)
		require.NoError(t, err)

		// Verify TypeA alarm no longer exists
		_, err = s.p.GetAlarm(t.Context(), alarm1Ref)
		require.ErrorIs(t, err, components.ErrNoAlarm)

		// Verify TypeB alarm still exists
		res, err := s.p.GetAlarm(t.Context(), alarm2Ref)
		require.NoError(t, err)
		assert.Contains(t, string(res.Data), "TypeB")
	})
}
