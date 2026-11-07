// Source: docklet-next/issues/358
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original v15 refusal through the real Direct sender and Edge tunnel receiver.

package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

func TestM20V15AgentsRefusedRealDirectAndEdge(t *testing.T) {
	fixture, err := os.ReadFile("../../../types/contract/testdata/agent-hub-v15.json")
	require.NoError(t, err)
	digest := sha256.Sum256(fixture)
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			f := newSignInRoutingFixture(t)
			var versions, commands atomic.Int32
			receiver := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "example", r.Header.Get("X-Arcane-Agent-Token"))
				switch r.URL.Path {
				case "/api/environments/0/system/health":
					w.WriteHeader(http.StatusNoContent)
				case "/api/app-version":
					versions.Add(1)
					require.NoError(t, json.NewEncoder(w).Encode(version.Info{AgentContractVersion: 15, AgentContractFingerprint: hex.EncodeToString(digest[:])}))
				case "/api/system/agent/restart":
					commands.Add(1)
					w.WriteHeader(http.StatusAccepted)
					_, _ = w.Write([]byte(`{"instanceId":"example-instance"}`))
				default:
					http.NotFound(w, r)
				}
			})
			server := httptest.NewServer(receiver)
			t.Cleanup(server.Close)
			if isEdge {
				actualSettingsTunnel(t, receiver, "example")
			}
			require.NoError(t, f.db.Exec("INSERT INTO api_keys(id,name,key_hash,key_prefix,kind) VALUES('key','Example','example','example','environment')").Error)
			require.NoError(t, f.db.Create(&environment.Environment{ID: "remote", Name: "Example", ApiUrl: server.URL, IsEdge: isEdge, Enabled: true, AccessToken: new("example"), ApiKeyID: new("key")}).Error)
			service := environment.NewEnvironmentService(f.db, server.Client(), nil, nil, nil, nil)
			_, err := service.ExecuteRemoteRequest(t.Context(), "remote", "POST", "/api/system/agent/restart", nil)
			require.ErrorIs(t, err, environment.ErrAgentUpdateRequired)
			require.Positive(t, versions.Load(), "the real sender reached the v15 receiver")
			require.Zero(t, commands.Load(), "incompatible peers receive no restart command")
		})
	}
}
