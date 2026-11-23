// Source: docklet-next/issues/584
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original completed connection-check producer metadata persistence proof.

package environment_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

func TestConnectionHandlerPersistsCompletedAction(t *testing.T) {
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "host.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/app-version" {
			require.NoError(t, json.NewEncoder(w).Encode(version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}))
			return
		}
		_, _ = w.Write([]byte(`{"status":"UP"}`))
	}))
	t.Cleanup(agent.Close)
	require.NoError(t, db.Create(&environment.Environment{ID: "example-host", Name: "Example", ApiUrl: agent.URL, Enabled: true, Status: "pending", AccessToken: new("example")}).Error)
	env := environment.NewEnvironmentService(db, agent.Client(), nil, nil, nil, nil)
	h := environment.NewHandler(env, nil, nil, nil, nil, activity.NewActivityService(db, nil))
	out, err := h.TestConnection(t.Context(), &environment.TestConnectionInput{ID: "example-host"})
	require.NoError(t, err)
	require.Equal(t, "online", out.Body.Data.Status)
	var rows []activity.Activity
	require.NoError(t, db.Where("environment_id = ?", "example-host").Find(&rows).Error)
	require.Len(t, rows, 1)
	require.EqualValues(t, "success", rows[0].Status)
	require.Equal(t, "check_environment_connection", rows[0].Metadata["action"])
}
