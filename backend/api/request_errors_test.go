// Source: docklet-next/issues/358
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original serialized request-problem and durable refusal DTO declaration proof.

package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	dto "github.com/florianfinn/docklet-next/types/request"
	"github.com/stretchr/testify/require"
)

func TestM20RequestProblemDeclaredOnEnvironmentOperations(t *testing.T) {
	for _, cfg := range []*config.Config{{}, {AgentMode: true}, {EdgeAgent: true}} {
		api := setupAPIForSpecMode(cfg)
		encoded, err := json.Marshal(&dto.Problem{Code: dto.PathEncoded, Status: 400})
		require.NoError(t, err)
		var actual any
		require.NoError(t, json.Unmarshal(encoded, &actual))
		for path, item := range api.OpenAPI().Paths {
			if !strings.HasPrefix(path, "/environments/") {
				continue
			}
			for _, op := range []*huma.Operation{item.Get, item.Post, item.Put, item.Patch, item.Delete} {
				if op == nil {
					continue
				}
				response := op.Responses["400"]
				require.NotNil(t, response, op.OperationID)
				media := response.Content["application/json"]
				require.NotNil(t, media, op.OperationID)
				result := &huma.ValidateResult{}
				huma.Validate(api.OpenAPI().Components.Schemas, media.Schema, huma.NewPathBuffer(nil, 0), huma.ModeReadFromServer, actual, result)
				require.Empty(t, result.Errors, op.OperationID)
			}
		}
		schemas := api.OpenAPI().Components.Schemas.Map()
		require.NotContains(t, schemas["ActivityRuntimeSnapshot"].Properties["messageCode"].Enum, dto.PathEncoded)
		require.Contains(t, schemas["EnvironmentAgentRestartRun"].Properties["code"].Enum, "host.busy")
	}
}
