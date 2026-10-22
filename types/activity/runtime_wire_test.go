// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Bounded remote runtime implementation, active in production since #125.

package activity

import (
	"encoding/json/v2"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func runtimeWireIdentityFixture() RuntimeRunIdentity {
	return RuntimeRunIdentity{RuntimeExecutorIdentity: RuntimeExecutorIdentity{ManagerID: "example-manager", RegistrationID: "example-registration", ExecutorGeneration: "11111111-1111-4111-8111-111111111111"}, RunID: "22222222-2222-4222-8222-222222222222", RequestDigest: strings.Repeat("a", 64)}
}
func TestRuntimeWireStrictShapeDepthAndQuery(t *testing.T) {
	id := runtimeWireIdentityFixture()
	wire, err := json.Marshal(RuntimeExecutionEnvelope{Identity: id, Operation: RuntimeStartContainer, Target: RuntimeTarget{Kind: "container", ID: strings.Repeat("a", 64)}, Attribution: RuntimeAttribution{ID: "example-owner", Username: "Operator"}}, json.FormatNilSliceAsNull(true))
	require.NoError(t, err)
	var e RuntimeExecutionEnvelope
	require.NoError(t, DecodeRuntimeJSON(wire, &e))
	for _, bad := range []string{`null`, string(wire) + ` {}`, strings.Replace(string(wire), `,"parentProjectId":null`, "", 1), strings.Replace(string(wire), `"parentProjectId":null`, `"parentProjectId":null,"unknown":true`, 1), strings.Replace(string(wire), `"username":"Operator"`, `"username":null`, 1), strings.Replace(string(wire), `"operation":`, `"operation":"start-container","operation":`, 1), strings.Repeat("[", 40) + strings.Repeat("]", 40)} {
		var value RuntimeExecutionEnvelope
		require.Error(t, DecodeRuntimeJSON([]byte(bad), &value), bad)
	}
	parsed, err := ParseRuntimeIdentityQuery(id.RunID, RuntimeIdentityQuery(id))
	require.NoError(t, err)
	require.Equal(t, id, parsed)
	for _, bad := range []string{RuntimeIdentityQuery(id) + "&managerId=example-other", RuntimeIdentityQuery(id) + "&extra=1", "managerId=example-manager", "%ZZ"} {
		_, err := ParseRuntimeIdentityQuery(id.RunID, bad)
		require.Error(t, err)
	}
}
func TestRuntimeWireWindowRejectsLosslessGapsAndOversize(t *testing.T) {
	id := runtimeWireIdentityFixture()
	now := time.Now()
	d := &RuntimeExecutionDetail{Activity: RuntimeSnapshot{Activity: Activity{ID: id.RunID, EnvironmentID: "0", Status: StatusRunning, StartedAt: now, CreatedAt: now}, RuntimeKnowledge: RuntimeKnowledge{OutcomeState: RuntimeOutcomeKnown}}, Messages: []Message{}, StepTrace: []StepTraceEntry{}, Output: []RuntimeExecutionOutputLine{}, RetainedStartSeq: 1}
	require.NoError(t, ValidateRuntimeExecutionDetail(d, id.RunID))
	d.Output = []RuntimeExecutionOutputLine{{OutputLine: OutputLine{Timestamp: now, Text: "line"}, OutputSeq: 2}}
	d.RetainedStartSeq = 2
	require.Error(t, ValidateRuntimeExecutionDetail(d, id.RunID))
	d.OutputTruncated = true
	require.NoError(t, ValidateRuntimeExecutionDetail(d, id.RunID))
	d.Output = append(d.Output, RuntimeExecutionOutputLine{OutputLine: OutputLine{Timestamp: now, Text: "line"}, OutputSeq: 2})
	require.Error(t, ValidateRuntimeExecutionDetail(d, id.RunID))
	d.Output = d.Output[:1]
	d.Output[0].Text = strings.Repeat("x", RuntimeMaxOutputBytes)
	require.Error(t, ValidateRuntimeExecutionDetail(d, id.RunID))
}
