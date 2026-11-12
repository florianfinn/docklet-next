// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original real serialized host and agent-access error schema validation.

package api

import (
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	dto "github.com/florianfinn/docklet-next/types/environment"
	"github.com/stretchr/testify/require"
	"strconv"
	"strings"
	"testing"
)

func TestHostRefusalDeclarationsValidateActualSerializedEnvelopes(t *testing.T) {
	api := SetupAPIForSpec()
	ui := handlerutil.AgentVerdict(&remenv.StatusError{StatusCode: 403, Body: []byte(`{"code":"settings.ui-configuration-disabled","status":403}`)})
	invalid := handlerutil.AgentVerdict(remenv.ErrAgentInvalidToken)
	outdated := handlerutil.AgentVerdict(remenv.ErrAgentUpdateRequired)
	validate := func(t *testing.T, op *huma.Operation, status int, body any) {
		t.Helper()
		response := op.Responses[strconv.Itoa(status)]
		require.NotNil(t, response, "missing declared status")
		media := response.Content["application/problem+json"]
		require.NotNil(t, media, "missing declared problem envelope")
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		var value any
		require.NoError(t, json.Unmarshal(encoded, &value))
		result := &huma.ValidateResult{}
		huma.Validate(api.OpenAPI().Components.Schemas, media.Schema, huma.NewPathBuffer(nil, 0), huma.ModeReadFromServer, value, result)
		require.Empty(t, result.Errors, "%s %d serialized %s", op.OperationID, status, encoded)
	}
	for _, tc := range []struct {
		name, path, method string
		status             int
		body               any
	}{
		{"register-name", "/environments", "POST", 400, &dto.HostProblem{Code: "host.name-invalid", Status: 400}},
		{"rename-name", "/environments/{id}", "PUT", 409, &dto.HostProblem{Code: "host.name-taken", Status: 409}},
		{"check-offline", "/environments/{id}/test", "POST", 502, &dto.HostProblem{Code: "host.unreachable", Status: 502}},
		{"check-outdated", "/environments/{id}/test", "POST", 409, outdated},
		{"check-invalid", "/environments/{id}/test", "POST", 502, invalid},
		{"autoheal-read-ui", "/environments/{id}/auto-heal", "GET", 502, ui},
		{"autoheal-write-ui-local", "/environments/{id}/auto-heal", "PUT", 403, &dto.HostProblem{Code: "settings.ui-configuration-disabled", Status: 403}},
		{"autoheal-write-ui-remote", "/environments/{id}/auto-heal", "PUT", 502, ui},
		{"settings-write-ui-local", "/environments/{id}/settings", "PUT", 403, &dto.HostProblem{Code: "settings.ui-configuration-disabled", Status: 403}},
		{"settings-write-ui-remote", "/environments/{id}/settings", "PUT", 502, ui},
		{"restart-busy", "/environments/{id}/agent/restart", "POST", 409, &dto.HostProblem{Code: "host.busy", Status: 409}},
		{"restart-outdated", "/environments/{id}/agent/restart", "POST", 409, outdated},
		{"restart-invalid", "/environments/{id}/agent/restart", "POST", 502, invalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := api.OpenAPI().Paths[tc.path]
			var op *huma.Operation
			switch tc.method {
			case "POST":
				op = item.Post
			case "PUT":
				op = item.Put
			default:
				op = item.Get
			}
			validate(t, op, tc.status, tc.body)
		})
	}
	for _, item := range api.OpenAPI().Paths {
		if item.Post != nil && item.Post.Responses["202"] != nil && strings.HasPrefix(item.Post.Responses["202"].Description, "New admission only") {
			t.Run(item.Post.OperationID, func(t *testing.T) {
				validate(t, item.Post, 409, &dto.HostProblem{Code: "host.restarting", Status: 409})
			})
		}
	}
}
