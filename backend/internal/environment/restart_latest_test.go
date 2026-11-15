// Source: docklet-next/issues/370
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original tests for the latest agent restart read.

package environment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/environment"
	"github.com/stretchr/testify/require"
)

func latestRestartRouter(t *testing.T, h *EnvironmentHandler) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Restart", "0.0.0"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	registerHostRestart(api, h)
	return mux
}

func readLatestRestart(t *testing.T, mux *http.ServeMux, host string) *dto.AgentRestartRun {
	t.Helper()
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/environments/"+host+"/agent/restarts/latest", nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body struct {
		Data dto.LatestAgentRestart `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	return body.Data.Run
}

func seedRestart(t *testing.T, h *EnvironmentHandler, runID, host, state string, created time.Time, code *string) {
	t.Helper()
	row := AgentRestartRequest{RunID: runID, EnvironmentID: host, RegistrationID: host, OldInstance: "instance", State: state, CreatedAt: created, UpdatedAt: created, Deadline: created.Add(3 * time.Minute), Code: code}
	if state == "connected" || state == "failed" || state == "unknown" {
		row.SettledAt = new(created.Add(time.Minute))
	}
	require.NoError(t, h.environmentService.db.Create(&row).Error)
}

// Row 1: the active restart wins even over a settled one created later; otherwise the newest
// settled run, with the run ID breaking ties; other hosts never; an empty body when none.
func TestLatestAgentRestartSelection(t *testing.T) {
	h := &EnvironmentHandler{environmentService: NewEnvironmentService(migratedHostDB(t), nil, nil, nil, nil, nil)}
	mux := latestRestartRouter(t, h)
	base := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	require.Nil(t, readLatestRestart(t, mux, "host-a"))

	busy := "host.busy"
	seedRestart(t, h, "run-active", "host-a", "sent", base, nil)
	seedRestart(t, h, "run-settled-later", "host-a", "failed", base.Add(time.Hour), &busy)
	seedRestart(t, h, "run-other-host", "host-b", "connected", base.Add(2*time.Hour), nil)
	run := readLatestRestart(t, mux, "host-a")
	require.NotNil(t, run)
	require.Equal(t, "run-active", run.RunID)
	require.Equal(t, "sent", run.State)

	require.NoError(t, h.environmentService.db.Model(&AgentRestartRequest{}).Where("run_id = ?", "run-active").Updates(map[string]any{"state": "connected", "settled_at": base.Add(time.Minute)}).Error)
	run = readLatestRestart(t, mux, "host-a")
	require.Equal(t, "run-settled-later", run.RunID)
	require.Equal(t, "host.busy", run.Code)
	require.NotNil(t, run.SettledAt)

	seedRestart(t, h, "run-tie-a", "host-c", "connected", base, nil)
	seedRestart(t, h, "run-tie-b", "host-c", "connected", base, nil)
	require.Equal(t, "run-tie-b", readLatestRestart(t, mux, "host-c").RunID)
}
