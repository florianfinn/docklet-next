// Source: docklet-next/issues/309
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original bounded allowlist for agent refusals.

package remenv

import (
	"encoding/json"
	"io"
)

const AgentRefusalBodyLimit = 16 << 10

// ReadAgentRefusalBody never reads an unbounded refusal body from an upstream.
func ReadAgentRefusalBody(body io.Reader) []byte {
	if body == nil {
		return nil
	}
	value, err := io.ReadAll(io.LimitReader(body, AgentRefusalBodyLimit+1))
	if err != nil || len(value) > AgentRefusalBodyLimit {
		return nil
	}
	return value
}

// ClassifyAgentRefusal returns only public host-access codes. The caller keeps
// upstream 401/403 behind a 502 envelope to preserve the browser's hub identity.
func ClassifyAgentRefusal(status int, body []byte) string {
	if !AgentAuthRejected(status) {
		return ""
	}
	var problem struct {
		Code string `json:"code"`
	}
	if len(body) > AgentRefusalBodyLimit || json.Unmarshal(body, &problem) != nil {
		return "agent.invalid-token"
	}
	if status == 403 && problem.Code == "settings.ui-configuration-disabled" {
		return "agent.ui-configuration-disabled"
	}
	if problem.Code == "agent.update-required" {
		return problem.Code
	}
	return "agent.invalid-token"
}
