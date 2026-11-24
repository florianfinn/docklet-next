// Source: docklet-next/issues/601
// Revision: 393e645a79236449c35cb8e1a4827c83bbf45da0
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original wire test for the agent effective IDs under the hub's JSON format.

package version

import (
	jsonv1 "encoding/json"
	json "encoding/json/v2"
	"testing"
)

// F2 (#601): root (0) stays on the wire; only an unknown ID is omitted. Options as backend/api/json_serializer.go.
func TestOnboardingAgentIDsKeepRoot(t *testing.T) {
	zero := uint32(0)
	body, err := json.Marshal(OnboardingChecks{AgentUID: &zero, AgentGID: &zero}, jsonv1.FormatDurationAsNano(true))
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		t.Fatal(err)
	}
	if root["agentUid"] != float64(0) || root["agentGid"] != float64(0) {
		t.Fatalf("root IDs must be encoded, got %s", body)
	}
	body, err = json.Marshal(OnboardingChecks{}, jsonv1.FormatDurationAsNano(true))
	if err != nil {
		t.Fatal(err)
	}
	var unknown map[string]any
	if err := json.Unmarshal(body, &unknown); err != nil {
		t.Fatal(err)
	}
	if _, ok := unknown["agentUid"]; ok {
		t.Fatalf("unknown IDs must be omitted, got %s", body)
	}
}
