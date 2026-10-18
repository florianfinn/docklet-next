// Source: docklet-next/docs/design/runtime-control.md
// Revision: 687e1abd5ed1be0e69a7fdc4c4677da2db78b85d
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Runtime serialization and unchanged legacy projection proofs.

package activity_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestRuntimeSnapshotKnowledgeIndependentOfStatus(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  activity.Status
		outcome activity.RuntimeOutcomeState
		cancel  bool
	}{
		{"unanswered", activity.StatusRunning, activity.RuntimeOutcomeUnknown, false},
		{"cancel_requested", activity.StatusRunning, activity.RuntimeOutcomeUnknown, true},
		{"manual_release", activity.StatusFailed, activity.RuntimeOutcomeUnknown, true},
		{"settled_interruption", activity.StatusFailed, activity.RuntimeOutcomeKnown, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := activity.RuntimeSnapshot{
				Activity: activity.Activity{ID: "run", Status: test.status},
				RuntimeKnowledge: activity.RuntimeKnowledge{
					OutcomeState: test.outcome, CancelRequested: test.cancel,
				},
			}
			data, err := json.Marshal(snapshot)
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(data, &fields))
			require.JSONEq(t, `"`+string(test.outcome)+`"`, string(fields["outcomeState"]))
			wantCancel := "false"
			if test.cancel {
				wantCancel = "true"
			}
			require.Equal(t, wantCancel, string(fields["cancelRequested"]))
			require.NotContains(t, fields, "RuntimeKnowledge")
			require.NotContains(t, fields, "Activity")
			var restored activity.RuntimeSnapshot
			require.NoError(t, json.Unmarshal(data, &restored))
			require.Equal(t, snapshot, restored)
		})
	}
}

func TestRuntimeVocabularyLeavesLegacyActivityJSONUnchanged(t *testing.T) {
	stamp := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	legacy := activity.Activity{
		ID: "run", EnvironmentID: "0", Type: activity.TypeContainerStart,
		Status: activity.StatusRunning, MessageCode: string(activity.CodeRunStarted),
		StartedAt: stamp, CreatedAt: stamp,
	}
	data, err := json.Marshal(legacy)
	require.NoError(t, err)
	const expected = `{"id":"run","environmentId":"0","type":"container_start","status":"running","messageCode":"run.started","startedAt":"2026-10-19T12:00:00Z","createdAt":"2026-10-19T12:00:00Z"}`
	require.Equal(t, expected, string(data))
	var restored activity.Activity
	require.NoError(t, json.Unmarshal([]byte(expected), &restored))
	require.Equal(t, legacy, restored)
}

func TestRuntimeAdmissionRetainsAcceptedIdentityAndUnknownKnowledge(t *testing.T) {
	admission := activity.RuntimeAdmission{
		ActivityID: "run", Code: activity.CodeRunStarted,
		Activity: activity.RuntimeSnapshot{
			Activity:         activity.Activity{ID: "run", Status: activity.StatusRunning},
			RuntimeKnowledge: activity.RuntimeKnowledge{OutcomeState: activity.RuntimeOutcomeUnknown},
		},
	}
	data, err := json.Marshal(admission)
	require.NoError(t, err)
	var restored activity.RuntimeAdmission
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, admission, restored)
	require.Equal(t, restored.ActivityID, restored.Activity.ID)
	require.Equal(t, activity.RuntimeOutcomeUnknown, restored.Activity.OutcomeState)
	// Declaring runtime codes must not expand the current public catalogue.
	for _, code := range []activity.Code{
		activity.CodeRunRequestMismatch, activity.CodeRuntimeOutcomeUnknown,
		activity.CodeRuntimeInterrupted, activity.CodeRuntimeCancelled, activity.CodeRuntimeFailed,
		activity.CodeRuntimeWaitTimeout, activity.CodeRuntimeWaiterDisconnected,
		activity.CodeRuntimeParentUnresolved, activity.CodeRuntimeTargetRemoved,
	} {
		require.False(t, activity.Valid(string(code)), string(code))
	}
}
