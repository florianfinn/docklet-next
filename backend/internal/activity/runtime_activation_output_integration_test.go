// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Registered legacy streams preserve output and encode visible loss as valid NDJSON.

package activity_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRuntimeActivationLegacyLossMarkerIsNDJSON(t *testing.T) {
	for _, transport := range []string{"local", "direct", "edge"} {
		for _, op := range []activity.RuntimeOperation{activity.RuntimeDeployProject, activity.RuntimeRedeployProject} {
			t.Run(transport+"/"+string(op), func(t *testing.T) {
				hub := activationApp(t, false, "")
				executor, env := hub, "0"
				if transport != "local" {
					executor = activationApp(t, true, uuid.NewString())
					env = activationRemote(t, hub, executor, transport)
				}
				suffix, body := activationOperation(executor.services, op)
				path, key := "/api/environments/"+env+suffix, uuid.NewString()
				status, response := activationCall(t, hub, path, key, "respond-async", body)
				require.Equal(t, 202, status, string(response))
				run := activationRunID(t, response)
				activationJoinRun(t, hub, executor, env, run)
				before := runtimeEffectEvents(executor.services.http.snapshot())
				const original = `{ "log": "retained progress", "example": true }`
				line := activitytypes.OutputLine{Timestamp: time.Now().UTC(), Stream: "stdout", Text: original}
				window, err := json.Marshal([]activitytypes.RuntimeExecutionOutputLine{{OutputLine: line, OutputSeq: 2}})
				require.NoError(t, err)
				require.NoError(t, hub.services.db.Model(&activity.RuntimeReceipt{}).Where("run_id = ?", run).UpdateColumn("output_next_seq", 3).Error)
				require.NoError(t, hub.services.db.Model(&activity.Activity{}).Where("id = ?", run).UpdateColumns(map[string]any{"runtime_output": string(window), "output": activity.OutputLines{line}, "output_truncated": true}).Error)
				status, response = activationCall(t, hub, path, key, "", body)
				require.Equal(t, 200, status, string(response))
				lines := strings.Split(strings.TrimSpace(string(response)), "\n")
				require.Len(t, lines, 4, string(response))
				frames := make([]map[string]any, len(lines))
				for i, line := range lines {
					require.NoError(t, json.Unmarshal([]byte(line), &frames[i]), "every legacy stream line must remain JSON")
				}
				require.Equal(t, "activity", frames[0]["type"])
				require.Equal(t, run, frames[0]["activityId"])
				require.Equal(t, "[runtime output truncated]", frames[1]["log"])
				require.Equal(t, original, lines[2], "existing JSON output payload bytes stay unchanged")
				require.Equal(t, true, frames[3]["done"])
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				require.NoError(t, hub.remote.WaitDispatch(ctx, run))
				cancel()
				require.Equal(t, before, runtimeEffectEvents(executor.services.http.snapshot()))
			})
		}
	}
}
