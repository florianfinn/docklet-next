// Source: docklet-next/issues/309
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original bounded agent refusal classification tests.

package remenv

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestClassifyAgentRefusalBoundedAllowlist(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body, code string
	}{
		{403, `{"code":"settings.ui-configuration-disabled"}`, "agent.ui-configuration-disabled"},
		{401, `{"code":"settings.ui-configuration-disabled"}`, "agent.invalid-token"},
		{403, `{"code":"agent.update-required"}`, "agent.update-required"},
		{401, `{"code":"agent.update-required"}`, "agent.update-required"},
		{403, `{"code":"arbitrary"}`, "agent.invalid-token"}, {401, `not JSON`, "agent.invalid-token"},
		{403, `{"code":123}`, "agent.invalid-token"}, {401, ``, "agent.invalid-token"}, {200, `{}`, ""},
	} {
		require.Equal(t, tc.code, ClassifyAgentRefusal(tc.status, []byte(tc.body)))
	}
	require.Empty(t, ReadAgentRefusalBody(nil))
	oversized := strings.Repeat(" ", AgentRefusalBodyLimit) + `{"code":"agent.update-required"}`
	require.Empty(t, ReadAgentRefusalBody(strings.NewReader(oversized)))
	require.Equal(t, "agent.invalid-token", ClassifyAgentRefusal(403, []byte(oversized)))
	status, code := AgentAccessFailure(&StatusError{StatusCode: 403, Body: []byte(`{"code":"settings.ui-configuration-disabled"}`)})
	require.Equal(t, 502, status)
	require.Equal(t, "agent.ui-configuration-disabled", code)
}
