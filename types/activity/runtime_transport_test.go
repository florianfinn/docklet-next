// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant transport JSON shape and legacy compatibility proofs only.

package activity_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/project"
	"github.com/stretchr/testify/require"
)

func transportIdentity() activity.RuntimeRunIdentity {
	return activity.RuntimeRunIdentity{
		RuntimeExecutorIdentity: activity.RuntimeExecutorIdentity{
			ManagerID: "example-manager", RegistrationID: "example-registration", ExecutorGeneration: "example-executor-generation",
		},
		RunID: "00000000-0000-4000-8000-000000000002", RequestDigest: strings.Repeat("a", 64),
	}
}

func TestRuntimeTransportExecutionJSON(t *testing.T) {
	envelope := activity.RuntimeExecutionEnvelope{
		Identity: transportIdentity(), Operation: activity.RuntimeDeployProject,
		Target:      activity.RuntimeTarget{Kind: "stack", ID: "example-stack"},
		Options:     activity.RuntimeOptions{Deploy: &project.DeployOptions{PullPolicy: "missing", ForceRecreate: true, RecreateVolumes: true}},
		Attribution: activity.RuntimeAttribution{ID: "example-actor", Username: "Operator", DisplayName: new("Original operator")},
	}
	data, err := json.Marshal(envelope)
	require.NoError(t, err)
	const expected = `{"identity":{"managerId":"example-manager","registrationId":"example-registration","executorGeneration":"example-executor-generation","runId":"00000000-0000-4000-8000-000000000002","requestDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"operation":"deploy-project","target":{"kind":"stack","id":"example-stack","parentProjectId":null},"options":{"deploy":{"pullPolicy":"missing","forceRecreate":true,"recreateVolumes":true},"restartServices":null},"attribution":{"id":"example-actor","username":"Operator","displayName":"Original operator"}}`
	require.Equal(t, expected, string(data))
	var restored activity.RuntimeExecutionEnvelope
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, envelope, restored)
	// The reused DeployOptions omits false fields; decode restores all four.
	require.Equal(t, project.DeployOptions{PullPolicy: "missing", ForceRecreate: true, RemoveOrphans: false, RecreateVolumes: true}, *restored.Options.Deploy)
	for _, forbidden := range []string{"key", "clientIdempotencyKey", "idempotencyKey", "issuedAt", "expiresAt", "deadline", "authorized", "isAdmin", "preferences", "roles", "grants", "token", "secret", "storeGeneration", "environmentId", "metadata"} {
		require.NotContains(t, string(data), `"`+forbidden+`"`)
	}
	for _, parent := range []*string{nil, new("example-parent-a"), new("example-parent-b")} {
		envelope.Operation = activity.RuntimeStartContainer
		envelope.Target = activity.RuntimeTarget{Kind: "container", ID: "example-container", ParentProjectID: parent}
		envelope.Options = activity.RuntimeOptions{}
		data, err := json.Marshal(envelope)
		require.NoError(t, err)
		require.Contains(t, string(data), `"parentProjectId":`)
		require.NoError(t, json.Unmarshal(data, &restored))
		require.Equal(t, parent, restored.Target.ParentProjectID)
	}
}

func TestRuntimeTransportOptionPresence(t *testing.T) {
	for _, test := range []struct {
		options activity.RuntimeOptions
		json    string
	}{
		{activity.RuntimeOptions{}, `{"deploy":null,"restartServices":null}`},
		{activity.RuntimeOptions{Deploy: &project.DeployOptions{}}, `{"deploy":{},"restartServices":null}`},
		{activity.RuntimeOptions{RestartServices: []string{}}, `{"deploy":null,"restartServices":[]}`},
		{activity.RuntimeOptions{RestartServices: []string{"api", "worker"}}, `{"deploy":null,"restartServices":["api","worker"]}`},
	} {
		data, err := json.Marshal(test.options)
		require.NoError(t, err)
		require.Equal(t, test.json, string(data))
		var restored activity.RuntimeOptions
		require.NoError(t, json.Unmarshal(data, &restored))
		require.Equal(t, test.options, restored)
	}
}

func TestRuntimeTransportOutcomeShapesAndLegacyJSON(t *testing.T) {
	stamp := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	legacy := activity.Activity{ID: transportIdentity().RunID, EnvironmentID: "0", Type: activity.TypeContainerStart,
		Status: activity.StatusRunning, StartedAt: stamp, CreatedAt: stamp, Progress: new(50), Step: "start",
		MessageCode: string(activity.CodeRunStarted), AffectedServices: []string{"api"},
		StartedBy: &activity.StartedBy{UserID: "example-actor", Username: "Operator"},
	}
	line := activity.OutputLine{Timestamp: stamp, Stream: "stdout", Text: "example output", Step: "start", Level: activity.MessageLevelInfo}
	data, err := json.Marshal(line)
	require.NoError(t, err)
	const legacyLine = `{"timestamp":"2026-10-19T12:00:00Z","stream":"stdout","text":"example output","step":"start","level":"info"}`
	require.Equal(t, legacyLine, string(data))
	detail := activity.Detail{Activity: legacy, Messages: []activity.Message{}, StepTrace: []activity.StepTraceEntry{}, Output: []activity.OutputLine{line}}
	before, err := json.Marshal(detail)
	require.NoError(t, err)
	for _, forbidden := range []string{"outcomeState", "cancelRequested", "outputSeq", "retainedStartSeq"} {
		require.NotContains(t, string(before), forbidden)
	}
	message := activity.Message{ID: "example-message", ActivityID: legacy.ID, Level: activity.MessageLevelInfo,
		Message: "Workflow started", Code: string(activity.CodeRunStarted), Step: "start", CreatedAt: stamp}
	step := activity.StepTraceEntry{ID: "start", Name: "Start", Status: activity.StatusRunning, Code: string(activity.CodeRunStarted), StartedAt: stamp}
	for _, test := range []struct {
		state  activity.RuntimeAnswerState
		status activity.Status
	}{
		{activity.RuntimeAnswerActive, activity.StatusRunning},
		{activity.RuntimeAnswerTerminal, activity.StatusSuccess},
		{activity.RuntimeAnswerTerminal, activity.StatusFailed},
		{activity.RuntimeAnswerTerminal, activity.StatusCancelled},
		{activity.RuntimeAnswerNotReceived, ""},
	} {
		t.Run(string(test.state)+string(test.status), func(t *testing.T) {
			outcome := activity.RuntimeExecutionOutcome{Identity: transportIdentity(), State: test.state}
			if test.state != activity.RuntimeAnswerNotReceived {
				snapshot := activity.RuntimeSnapshot{Activity: legacy, RuntimeKnowledge: activity.RuntimeKnowledge{OutcomeState: activity.RuntimeOutcomeKnown}}
				snapshot.Status = test.status
				outcome.Detail = &activity.RuntimeExecutionDetail{
					Activity: snapshot, Messages: []activity.Message{message}, StepTrace: []activity.StepTraceEntry{step},
					Output: []activity.RuntimeExecutionOutputLine{{OutputLine: line, OutputSeq: 41}}, OutputTruncated: true, RetainedStartSeq: 41,
				}
			}
			data, err := json.Marshal(outcome)
			require.NoError(t, err)
			var restored activity.RuntimeExecutionOutcome
			require.NoError(t, json.Unmarshal(data, &restored))
			require.Equal(t, outcome, restored)
			require.Equal(t, transportIdentity(), restored.Identity)
			if test.state == activity.RuntimeAnswerNotReceived {
				require.NotContains(t, string(data), `"detail"`)
				require.Contains(t, string(data), `"state":"not-received"`)

			} else {
				require.Contains(t, string(data), `"outputSeq":41`)
				require.Contains(t, string(data), `"retainedStartSeq":41`)
				require.Contains(t, string(data), `"outcomeState":"known"`)
				require.Contains(t, string(data), `"cancelRequested":false`)
				require.NotContains(t, string(data), `"OutputLine"`)
				require.Equal(t, restored.Identity.RunID, restored.Detail.Activity.ID)
				require.Equal(t, "0", restored.Detail.Activity.EnvironmentID)
				require.Equal(t, activity.RuntimeOutcomeKnown, restored.Detail.Activity.OutcomeState)
				require.EqualValues(t, 41, restored.Detail.Output[0].OutputSeq)
				require.EqualValues(t, 41, restored.Detail.RetainedStartSeq)
				require.Equal(t, line, restored.Detail.Output[0].OutputLine)
			}
		})
	}
	// Runtime wrapper creation/serialization does not alter registered Detail/OutputLine.
	after, err := json.Marshal(detail)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
	for _, value := range []string{"active", "terminal", "not-received", "deploy-project", "down-project", "restart-project", "redeploy-project", "start-container", "stop-container", "restart-container"} {
		require.False(t, activity.Valid(value))
	}
}

func TestRuntimeTransportEmptyWindowCursor(t *testing.T) {
	detail := activity.RuntimeExecutionDetail{Output: []activity.RuntimeExecutionOutputLine{}, OutputTruncated: true, RetainedStartSeq: 42}
	data, err := json.Marshal(detail)
	require.NoError(t, err)
	require.Contains(t, string(data), `"output":[]`)
	require.Contains(t, string(data), `"retainedStartSeq":42`)
	var restored activity.RuntimeExecutionDetail
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, detail, restored)
	// This only pins the carrier. C5 proves sequencing, eviction and window byte bounds.
}
