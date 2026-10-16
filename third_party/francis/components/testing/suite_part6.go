// Source: italypaleale/francis/components/testing/suite.go
// Revision: 23cd46831a543a73eec05ad034afd46245225876
// Retrieved: 2026-10-16
// License: MIT
// Deviations: Pinned provider test support restored for default tests; reserved example addresses; oversized suite split without behavior changes.

package comptesting

import (
	"context"
	"fmt"
	"github.com/italypaleale/francis/components"
	"github.com/italypaleale/francis/internal/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func (s Suite) TestWorkflowLabels(t *testing.T) {
	// setState stores state and workflow labels for an actor of the given type
	// A nil labels argument stores none, which is how the write clears whatever the row had
	setState := func(t *testing.T, ctx context.Context, actorType string, actorID string, labels *components.WorkflowLabels) {
		t.Helper()
		err := s.p.SetState(ctx, ref.ActorRef{ActorType: actorType, ActorID: actorID}, []byte("data"), components.SetStateOpts{WorkflowLabels: labels})
		require.NoError(t, err)
	}

	// listIDs returns the actor IDs in a page, so tests can assert on the ordering without repeating the mapping
	listIDs := func(t *testing.T, ctx context.Context, req components.ListStatesReq) []string {
		t.Helper()
		res, err := s.p.ListStates(ctx, req)
		require.NoError(t, err)

		ids := make([]string, len(res.States))
		for i, state := range res.States {
			ids[i] = state.ActorID
		}
		return ids
	}

	// Seed with empty database
	require.NoError(t, s.p.Seed(t.Context(), Spec{}))

	t.Run("filters a listing on one field", func(t *testing.T) {
		ctx := t.Context()

		setState(t, ctx, "LabelFilter", "actor-01", &components.WorkflowLabels{Status: "running", Version: 1})
		setState(t, ctx, "LabelFilter", "actor-02", &components.WorkflowLabels{Status: "done", Version: 1})
		setState(t, ctx, "LabelFilter", "actor-03", &components.WorkflowLabels{Status: "running", Version: 1})

		ids := listIDs(t, ctx, components.ListStatesReq{ActorType: "LabelFilter", WorkflowLabels: &components.WorkflowLabels{Status: "running"}})
		assert.Equal(t, []string{"actor-01", "actor-03"}, ids)
	})

	t.Run("requires every field the filter sets to match", func(t *testing.T) {
		ctx := t.Context()

		setState(t, ctx, "LabelMulti", "actor-01", &components.WorkflowLabels{Status: "running", Version: 1})
		setState(t, ctx, "LabelMulti", "actor-02", &components.WorkflowLabels{Status: "running", Version: 2})
		setState(t, ctx, "LabelMulti", "actor-03", &components.WorkflowLabels{Status: "done", Version: 2})

		ids := listIDs(t, ctx, components.ListStatesReq{ActorType: "LabelMulti", WorkflowLabels: &components.WorkflowLabels{Status: "running", Version: 2}})
		assert.Equal(t, []string{"actor-02"}, ids)

		// A field the row does not carry at all excludes it, just like a mismatched value
		ids = listIDs(t, ctx, components.ListStatesReq{ActorType: "LabelMulti", WorkflowLabels: &components.WorkflowLabels{Parent: "nobody"}})
		assert.Empty(t, ids)
	})

	t.Run("filters on the parent of a child instance", func(t *testing.T) {
		ctx := t.Context()

		setState(t, ctx, "LabelParent", "child-01", &components.WorkflowLabels{Status: "running", Version: 1, Parent: "p1"})
		setState(t, ctx, "LabelParent", "child-02", &components.WorkflowLabels{Status: "running", Version: 1, Parent: "p2"})
		setState(t, ctx, "LabelParent", "top-01", &components.WorkflowLabels{Status: "running", Version: 1})

		ids := listIDs(t, ctx, components.ListStatesReq{ActorType: "LabelParent", WorkflowLabels: &components.WorkflowLabels{Parent: "p1"}})
		assert.Equal(t, []string{"child-01"}, ids)
	})

	t.Run("an unfiltered listing returns every actor regardless of labels", func(t *testing.T) {
		ctx := t.Context()

		setState(t, ctx, "LabelUnfiltered", "actor-01", &components.WorkflowLabels{Status: "running", Version: 1})
		setState(t, ctx, "LabelUnfiltered", "actor-02", nil)

		ids := listIDs(t, ctx, components.ListStatesReq{ActorType: "LabelUnfiltered"})
		assert.Equal(t, []string{"actor-01", "actor-02"}, ids)

		// A filter with no field set is the same as no filter at all, rather than matching nothing
		ids = listIDs(t, ctx, components.ListStatesReq{ActorType: "LabelUnfiltered", WorkflowLabels: &components.WorkflowLabels{}})
		assert.Equal(t, []string{"actor-01", "actor-02"}, ids)
	})

	t.Run("a later write replaces the previous labels", func(t *testing.T) {
		ctx := t.Context()

		setState(t, ctx, "LabelReplace", "actor-01", &components.WorkflowLabels{Status: "running", Version: 1})
		setState(t, ctx, "LabelReplace", "actor-01", &components.WorkflowLabels{Status: "done", Version: 1})

		assert.Empty(t, listIDs(t, ctx, components.ListStatesReq{ActorType: "LabelReplace", WorkflowLabels: &components.WorkflowLabels{Status: "running"}}))
		assert.Equal(t, []string{"actor-01"}, listIDs(t, ctx, components.ListStatesReq{ActorType: "LabelReplace", WorkflowLabels: &components.WorkflowLabels{Status: "done"}}))

		// Writing with no labels at all clears what the row had
		setState(t, ctx, "LabelReplace", "actor-01", nil)
		assert.Empty(t, listIDs(t, ctx, components.ListStatesReq{ActorType: "LabelReplace", WorkflowLabels: &components.WorkflowLabels{Status: "done"}}))
		assert.Equal(t, []string{"actor-01"}, listIDs(t, ctx, components.ListStatesReq{ActorType: "LabelReplace"}))
	})

	t.Run("labels do not outlive the state they describe", func(t *testing.T) {
		ctx := t.Context()

		setState(t, ctx, "LabelDeleted", "actor-01", &components.WorkflowLabels{Status: "running", Version: 1})
		require.NoError(t, s.p.DeleteState(ctx, ref.ActorRef{ActorType: "LabelDeleted", ActorID: "actor-01"}))

		// The state is gone, so a filtered listing must not resurrect it
		assert.Empty(t, listIDs(t, ctx, components.ListStatesReq{ActorType: "LabelDeleted", WorkflowLabels: &components.WorkflowLabels{Status: "running"}}))

		// And writing the actor again with no labels must not inherit the old ones
		setState(t, ctx, "LabelDeleted", "actor-01", nil)
		assert.Empty(t, listIDs(t, ctx, components.ListStatesReq{ActorType: "LabelDeleted", WorkflowLabels: &components.WorkflowLabels{Status: "running"}}))
	})

	t.Run("a filtered listing pages in actor-ID order", func(t *testing.T) {
		ctx := t.Context()

		for i := 1; i <= 6; i++ {
			labels := &components.WorkflowLabels{Status: "done", Version: 1}
			if i%2 == 1 {
				labels = &components.WorkflowLabels{Status: "running", Version: 1}
			}
			setState(t, ctx, "LabelPaged", fmt.Sprintf("actor-%02d", i), labels)
		}

		var (
			seen   []string
			cursor string
		)
		for {
			res, err := s.p.ListStates(ctx, components.ListStatesReq{ActorType: "LabelPaged", WorkflowLabels: &components.WorkflowLabels{Status: "running"}, After: cursor, Limit: 2})
			require.NoError(t, err)
			for _, state := range res.States {
				seen = append(seen, state.ActorID)
			}

			if !res.HasMore || len(res.States) == 0 {
				break
			}
			cursor = res.States[len(res.States)-1].ActorID
		}
		assert.Equal(t, []string{"actor-01", "actor-03", "actor-05"}, seen)
	})
}

func (s Suite) TestGetAlarm(t *testing.T) {
	t.Run("get alarm from test spec", func(t *testing.T) {
		// Seed with test data
		err := s.p.Seed(t.Context(), GetSpec())
		require.NoError(t, err)

		// Get an existing alarm from the test spec
		alarmRef := ref.AlarmRef{
			ActorType: "A",
			ActorID:   "A-1",
			Name:      "Alarm-A-1",
		}

		res, err := s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)

		// Verify it exists (exact values depend on the test spec)
		assert.False(t, res.DueTime.IsZero())
		assert.Equal(t, []byte("active-A-1"), res.Data)
	})

	t.Run("get existing alarm with all fields", func(t *testing.T) {
		// Seed with empty database
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)

		// Create an alarm with all fields
		alarmRef := ref.AlarmRef{
			ActorType: "TestActor",
			ActorID:   "test-id",
			Name:      "test-alarm",
		}

		alarmData := []byte(`{"message": "test data"}`)
		now := time.Now()
		dueTime := now.Add(1 * time.Hour)
		ttl := now.Add(24 * time.Hour)

		setReq := components.SetAlarmReq{
			DueTime:  dueTime,
			Interval: "1h",
			TTL:      &ttl,
			Data:     alarmData,
		}

		lease, err := s.p.SetAlarm(t.Context(), alarmRef, setReq)
		require.NoError(t, err)
		assert.Nil(t, lease)

		// Get the alarm
		res, err := s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)

		// Verify all fields
		assert.WithinDuration(t, dueTime, res.DueTime, time.Second)
		assert.Equal(t, "1h", res.Interval)
		require.NotNil(t, res.TTL)
		assert.WithinDuration(t, ttl, *res.TTL, time.Second)
		assert.Equal(t, alarmData, res.Data)
	})

	t.Run("get existing alarm with minimal fields", func(t *testing.T) {
		// Seed with empty database
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)

		// Create an alarm with only required fields
		alarmRef := ref.AlarmRef{
			ActorType: "TestActor",
			ActorID:   "test-id-minimal",
			Name:      "test-alarm-minimal",
		}

		now := time.Now()
		dueTime := now.Add(1 * time.Hour)

		setReq := components.SetAlarmReq{
			AlarmProperties: ref.AlarmProperties{
				DueTime: dueTime,
				// Leave Interval, TTL, and Data as defaults
			},
		}

		_, err = s.p.SetAlarm(t.Context(), alarmRef, setReq)
		require.NoError(t, err)

		// Get the alarm
		res, err := s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)

		// Verify fields
		assert.WithinDuration(t, dueTime, res.DueTime, time.Second)
		assert.Empty(t, res.Interval)
		assert.Nil(t, res.TTL)
		assert.Nil(t, res.Data)
	})

	t.Run("returns ErrNoAlarm for non-existent alarm", func(t *testing.T) {
		// Seed with empty database
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)

		// Try to get a non-existent alarm
		alarmRef := ref.AlarmRef{
			ActorType: "NonExistentActor",
			ActorID:   "non-existent-id",
			Name:      "non-existent-alarm",
		}

		_, err = s.p.GetAlarm(t.Context(), alarmRef)
		require.ErrorIs(t, err, components.ErrNoAlarm)
	})
}

func (s Suite) TestSetAlarm(t *testing.T) {
	t.Run("create new alarm with all fields", func(t *testing.T) {
		// Seed with empty database
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)

		// Create an alarm with all fields
		alarmRef := ref.AlarmRef{
			ActorType: "TestActor",
			ActorID:   "test-id",
			Name:      "test-alarm",
		}

		alarmData := []byte(`{"message": "test data"}`)
		now := time.Now()
		dueTime := now.Add(1 * time.Hour)
		ttl := now.Add(24 * time.Hour)

		setReq := components.SetAlarmReq{
			DueTime:  dueTime,
			Interval: "1h",
			TTL:      &ttl,
			Data:     alarmData,
		}
		_, err = s.p.SetAlarm(t.Context(), alarmRef, setReq)
		require.NoError(t, err)

		// Verify the alarm was created by getting it
		res, err := s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)

		assert.WithinDuration(t, dueTime, res.DueTime, time.Second)
		assert.Equal(t, "1h", res.Interval)
		require.NotNil(t, res.TTL)
		assert.WithinDuration(t, ttl, *res.TTL, time.Second)
		assert.Equal(t, alarmData, res.Data)
	})

	t.Run("create new alarm with minimal fields", func(t *testing.T) {
		// Seed with empty database
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)

		// Create an alarm with only required fields
		alarmRef := ref.AlarmRef{
			ActorType: "TestActor",
			ActorID:   "test-id-minimal",
			Name:      "test-alarm-minimal",
		}

		now := time.Now()
		dueTime := now.Add(1 * time.Hour)

		setReq := components.SetAlarmReq{
			DueTime: dueTime,
		}

		_, err = s.p.SetAlarm(t.Context(), alarmRef, setReq)
		require.NoError(t, err)

		// Verify the alarm was created
		res, err := s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)

		assert.WithinDuration(t, dueTime, res.DueTime, time.Second)
		assert.Empty(t, res.Interval)
		assert.Nil(t, res.TTL)
		assert.Nil(t, res.Data)
	})

	t.Run("update existing alarm replaces all fields", func(t *testing.T) {
		// Seed with empty database
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)

		alarmRef := ref.AlarmRef{
			ActorType: "TestActor",
			ActorID:   "test-id",
			Name:      "test-alarm",
		}

		// Create initial alarm
		now := time.Now()
		initialDueTime := now.Add(1 * time.Hour)
		initialData := []byte(`{"version": 1}`)
		initialTTL := now.Add(12 * time.Hour)

		initialReq := components.SetAlarmReq{
			DueTime:  initialDueTime,
			Interval: "1h",
			TTL:      &initialTTL,
			Data:     initialData,
		}

		_, err = s.p.SetAlarm(t.Context(), alarmRef, initialReq)
		require.NoError(t, err)

		// Update the alarm with different values
		updatedDueTime := now.Add(2 * time.Hour)
		updatedData := []byte(`{"version": 2}`)
		updatedTTL := now.Add(24 * time.Hour)

		updateReq := components.SetAlarmReq{
			DueTime:  updatedDueTime,
			Interval: "2h",
			TTL:      &updatedTTL,
			Data:     updatedData,
		}

		_, err = s.p.SetAlarm(t.Context(), alarmRef, updateReq)
		require.NoError(t, err)

		// Verify the alarm was updated
		res, err := s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)

		assert.WithinDuration(t, updatedDueTime, res.DueTime, time.Second)
		assert.Equal(t, "2h", res.Interval)
		require.NotNil(t, res.TTL)
		assert.WithinDuration(t, updatedTTL, *res.TTL, time.Second)
		assert.Equal(t, updatedData, res.Data)
	})

	t.Run("update existing alarm clears optional fields when not provided", func(t *testing.T) {
		// Seed with empty database
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)

		alarmRef := ref.AlarmRef{
			ActorType: "TestActor",
			ActorID:   "test-id",
			Name:      "test-alarm",
		}

		// Create initial alarm with all fields
		now := time.Now()
		initialDueTime := now.Add(1 * time.Hour)
		initialData := []byte(`{"version": 1}`)
		initialTTL := now.Add(12 * time.Hour)

		initialReq := components.SetAlarmReq{
			DueTime:  initialDueTime,
			Interval: "1h",
			TTL:      &initialTTL,
			Data:     initialData,
		}

		_, err = s.p.SetAlarm(t.Context(), alarmRef, initialReq)
		require.NoError(t, err)

		// Update with minimal fields (should clear optional fields)
		updatedDueTime := now.Add(2 * time.Hour)

		updateReq := components.SetAlarmReq{
			AlarmProperties: ref.AlarmProperties{
				DueTime: updatedDueTime,
				// No Interval, TTL, or Data
			},
		}

		_, err = s.p.SetAlarm(t.Context(), alarmRef, updateReq)
		require.NoError(t, err)

		// Verify optional fields were cleared
		res, err := s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)

		assert.WithinDuration(t, updatedDueTime, res.DueTime, time.Second)
		assert.Empty(t, res.Interval)
		assert.Nil(t, res.TTL)
		assert.Nil(t, res.Data)
	})

	t.Run("set alarm with empty data array becomes nil", func(t *testing.T) {
		// Seed with empty database
		err := s.p.Seed(t.Context(), Spec{})
		require.NoError(t, err)

		alarmRef := ref.AlarmRef{
			ActorType: "TestActor",
			ActorID:   "test-id",
			Name:      "test-alarm",
		}

		now := time.Now()
		dueTime := now.Add(1 * time.Hour)

		setReq := components.SetAlarmReq{
			AlarmProperties: ref.AlarmProperties{
				DueTime: dueTime,
				Data:    []byte{}, // Empty but non-nil slice
			},
		}

		_, err = s.p.SetAlarm(t.Context(), alarmRef, setReq)
		require.NoError(t, err)

		// Verify empty data becomes nil
		res, err := s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)

		assert.Nil(t, res.Data) // Should be nil, not empty slice
	})

	t.Run("set alarm with identical properties preserves lease", func(t *testing.T) {
		// Seed with test data that includes leased alarms
		err := s.p.Seed(t.Context(), GetSpec())
		require.NoError(t, err)

		// Get an alarm that should have a lease (from the spec, C-001 through C-005 have valid leases)
		alarmRef := ref.AlarmRef{
			ActorType: "C",
			ActorID:   "C-001",
			Name:      "C-001",
		}

		// First, verify the alarm exists and get its current properties
		originalAlarm, err := s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)

		// Get all hosts/alarms to check the current lease status
		originalSpec, err := s.p.GetAllHosts(t.Context())
		require.NoError(t, err)

		// Find the original alarm in the spec to get lease information
		var originalAlarmSpec *AlarmSpec
		for i := range originalSpec.Alarms {
			alarm := &originalSpec.Alarms[i]
			if alarm.ActorType == alarmRef.ActorType &&
				alarm.ActorID == alarmRef.ActorID &&
				alarm.Name == alarmRef.Name {
				originalAlarmSpec = alarm
				break
			}
		}
		require.NotNil(t, originalAlarmSpec, "Original alarm should exist in spec")
		require.NotNil(t, originalAlarmSpec.LeaseID, "Original alarm should have a lease")

		// Now call SetAlarm with exactly the same properties
		setReq := components.SetAlarmReq{
			DueTime:  originalAlarm.DueTime,
			Interval: originalAlarm.Interval,
			TTL:      originalAlarm.TTL,
			Data:     originalAlarm.Data,
		}

		_, err = s.p.SetAlarm(t.Context(), alarmRef, setReq)
		require.NoError(t, err)

		// Get the alarm spec again to verify the lease is preserved
		updatedSpec, err := s.p.GetAllHosts(t.Context())
		require.NoError(t, err)

		// Find the updated alarm in the spec
		var updatedAlarmSpec *AlarmSpec
		for i := range updatedSpec.Alarms {
			alarm := &updatedSpec.Alarms[i]
			if alarm.ActorType == alarmRef.ActorType &&
				alarm.ActorID == alarmRef.ActorID &&
				alarm.Name == alarmRef.Name {
				updatedAlarmSpec = alarm
				break
			}
		}
		require.NotNil(t, updatedAlarmSpec, "Updated alarm should exist in spec")

		// Verify the lease is preserved (same lease ID and expiration)
		assert.NotNil(t, updatedAlarmSpec.LeaseID, "Lease should still exist")
		assert.Equal(t, *originalAlarmSpec.LeaseID, *updatedAlarmSpec.LeaseID, "Lease ID should be preserved")

		if originalAlarmSpec.LeaseExp != nil && updatedAlarmSpec.LeaseExp != nil {
			assert.Equal(t, *originalAlarmSpec.LeaseExp, *updatedAlarmSpec.LeaseExp, "Lease expiration should be preserved")
		}

		// Verify that the alarm properties are still the same
		updatedAlarm, err := s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)

		assert.WithinDuration(t, originalAlarm.DueTime, updatedAlarm.DueTime, time.Second/10)
		assert.Equal(t, originalAlarm.Interval, updatedAlarm.Interval)
		if originalAlarm.TTL != nil && updatedAlarm.TTL != nil {
			assert.WithinDuration(t, *originalAlarm.TTL, *updatedAlarm.TTL, time.Second/10)
		} else {
			assert.Equal(t, originalAlarm.TTL, updatedAlarm.TTL)
		}
		assert.Equal(t, originalAlarm.Data, updatedAlarm.Data)
	})

	t.Run("set alarm with different properties nullifies lease", func(t *testing.T) {
		// Seed with test data that includes leased alarms
		err := s.p.Seed(t.Context(), GetSpec())
		require.NoError(t, err)

		// Get an alarm that should have a lease (from the spec, C-002 has a valid lease)
		alarmRef := ref.AlarmRef{
			ActorType: "C",
			ActorID:   "C-002",
			Name:      "C-002",
		}

		// First, verify the alarm exists and has a lease
		originalAlarm, err := s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)

		// Get all hosts/alarms to check the current lease status
		originalSpec, err := s.p.GetAllHosts(t.Context())
		require.NoError(t, err)

		// Find the original alarm in the spec to get lease information
		var originalAlarmSpec *AlarmSpec
		for i := range originalSpec.Alarms {
			alarm := &originalSpec.Alarms[i]
			if alarm.ActorType == alarmRef.ActorType &&
				alarm.ActorID == alarmRef.ActorID &&
				alarm.Name == alarmRef.Name {
				originalAlarmSpec = alarm
				break
			}
		}
		require.NotNil(t, originalAlarmSpec, "Original alarm should exist in spec")
		require.NotNil(t, originalAlarmSpec.LeaseID, "Original alarm should have a lease")

		// Now call SetAlarm with different properties (change due time)
		newDueTime := originalAlarm.DueTime.Add(1 * time.Hour)
		setReq := components.SetAlarmReq{
			DueTime:  newDueTime,
			Interval: originalAlarm.Interval,
			TTL:      originalAlarm.TTL,
			Data:     originalAlarm.Data,
		}

		_, err = s.p.SetAlarm(t.Context(), alarmRef, setReq)
		require.NoError(t, err)

		// Get the alarm spec again to verify the lease is nullified
		updatedSpec, err := s.p.GetAllHosts(t.Context())
		require.NoError(t, err)

		// Find the updated alarm in the spec
		var updatedAlarmSpec *AlarmSpec
		for i := range updatedSpec.Alarms {
			alarm := &updatedSpec.Alarms[i]
			if alarm.ActorType == alarmRef.ActorType &&
				alarm.ActorID == alarmRef.ActorID &&
				alarm.Name == alarmRef.Name {
				updatedAlarmSpec = alarm
				break
			}
		}
		require.NotNil(t, updatedAlarmSpec, "Updated alarm should exist in spec")

		// Verify the lease is nullified
		assert.Nil(t, updatedAlarmSpec.LeaseID, "Lease should be nullified when properties change")
		assert.Nil(t, updatedAlarmSpec.LeaseExp, "Lease expiration should be nullified when properties change")

		// Verify that the alarm properties were updated
		updatedAlarm, err := s.p.GetAlarm(t.Context(), alarmRef)
		require.NoError(t, err)

		assert.WithinDuration(t, newDueTime, updatedAlarm.DueTime, time.Second/10)
	})

	t.Run("replacing an alarm invalidates a previously-issued lease", func(t *testing.T) {
		ctx := t.Context()

		// Seed with the test data
		require.NoError(t, s.p.Seed(ctx, GetSpec()))

		// Acquire a real lease on an upcoming alarm
		leased, err := s.p.FetchAndLeaseUpcomingAlarms(ctx, components.FetchAndLeaseUpcomingAlarmsReq{
			Hosts: []string{SpecHostH7, SpecHostH8},
		})
		require.NoError(t, err)
		require.NotEmpty(t, leased, "should have fetched and leased some alarms")

		oldLease := leased[0]
		aRef := oldLease.AlarmRef()

		// Sanity check: the lease is currently valid
		_, err = s.p.GetLeasedAlarm(ctx, oldLease)
		require.NoError(t, err)

		// Replace the alarm with different properties. This generates a new alarm ID
		// and must invalidate the prior lease (which referenced the old alarm ID).
		newDueTime := oldLease.DueTime().Add(1 * time.Hour)
		_, err = s.p.SetAlarm(ctx, aRef, components.SetAlarmReq{
			DueTime: newDueTime,
			Data:    []byte("replaced"),
		})
		require.NoError(t, err)

		// The previous lease must no longer resolve to anything, for any operation.
		// (Regression: a stale lease that still resolved could read/update/delete a
		// phantom alarm, and DeleteLeasedAlarm via the stale lease could even remove
		// the legitimate replacement.)
		_, err = s.p.GetLeasedAlarm(ctx, oldLease)
		require.ErrorIs(t, err, components.ErrNoAlarm, "stale lease should not resolve after the alarm was replaced")

		err = s.p.UpdateLeasedAlarm(ctx, oldLease, components.UpdateLeasedAlarmReq{DueTime: newDueTime})
		require.ErrorIs(t, err, components.ErrNoAlarm, "stale lease should not be usable to update the replaced alarm")

		err = s.p.DeleteLeasedAlarm(ctx, oldLease)
		require.ErrorIs(t, err, components.ErrNoAlarm, "stale lease should not be usable to delete the replaced alarm")

		// The replacement alarm still exists and reflects the new properties
		res, err := s.p.GetAlarm(ctx, aRef)
		require.NoError(t, err)
		assert.WithinDuration(t, newDueTime, res.DueTime, time.Second)
		assert.Equal(t, []byte("replaced"), res.Data)
	})
}
