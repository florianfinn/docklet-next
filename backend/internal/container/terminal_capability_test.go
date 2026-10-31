// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original D4 real state readers across origins and host availability.

package container

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

func TestShellCapabilityLocalAllOrigins(t *testing.T) {
	for _, origin := range []string{"compose", "standalone", "unraid", "unknown"} {
		t.Run(origin, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				labels := map[string]string{}
				if origin == "compose" {
					labels["com.docker.compose.project"] = "example"
				}
				if origin == "unraid" {
					labels["net.unraid.docker.icon"] = ""
				}
				_ = json.MarshalWrite(w, map[string]any{"Id": "example", "State": map[string]any{"Running": true}, "Config": map[string]any{"Labels": labels}})
			}))
			defer server.Close()
			cli, err := client.New(client.WithHost(strings.Replace(server.URL, "http://", "tcp://", 1)), client.WithAPIVersion("1.53"), client.WithHTTPClient(server.Client()))
			require.NoError(t, err)
			defer cli.Close()
			svc := &ContainerService{dockerService: (&docker.DockerClientService{}).WithClient(cli)}
			require.Empty(t, svc.ShellCapabilityReader(nil)(t.Context(), "0", "example"))
		})
	}
}
func TestShellCapabilityHostAvailabilityAndStopped(t *testing.T) {
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "capability.db")), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.Exec("INSERT INTO api_keys(id,name,key_hash,key_prefix,kind) VALUES(?,?,?,?,?)", "example-key", "Example", "test-only", "example", "environment").Error)
	var running bool
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/app-version":
			_ = json.MarshalWrite(w, version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
		case "/api/environments/0/system/health":
			w.WriteHeader(204)
		case "/api/environments/0/containers/missing":
			w.WriteHeader(http.StatusNotFound)
			_ = json.MarshalWrite(w, map[string]any{"success": false})
		case "/api/environments/0/containers/broken":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_ = json.MarshalWrite(w, map[string]any{"success": true, "data": map[string]any{"state": map[string]any{"running": running}}})
		}
	}))
	defer agent.Close()
	hosts := environment.NewEnvironmentService(db, http.DefaultClient, nil, nil, nil, nil)
	reader := (&ContainerService{}).ShellCapabilityReader(hosts)
	for i, tc := range []struct {
		status  string
		running bool
		reason  string
	}{
		{"online", true, ""}, {"standby", true, ""}, {"online", false, "container.shell-not-running"},
		{"offline", true, "agent.unavailable"}, {"pending", true, "agent.unavailable"},
		{string(environment.EnvironmentStatusTokenRejected), true, "agent.invalid-token"}, {"update-required", true, "agent.update-required"},
	} {
		id := string(rune('a' + i))
		running = tc.running
		require.NoError(t, db.Create(&environment.Environment{ID: id, Name: "Example " + id, ApiUrl: agent.URL, Status: tc.status, Enabled: true, AccessToken: new("test-only"), ApiKeyID: new("example-key")}).Error)
		require.Equal(t, tc.reason, reader(t.Context(), id, "example"), tc.status)
	}
	require.NoError(t, db.Create(&environment.Environment{ID: "z", Name: "Example z", ApiUrl: agent.URL, Status: "online", Enabled: true, AccessToken: new("test-only"), ApiKeyID: new("example-key")}).Error)
	require.Equal(t, "capability.unavailable", reader(t.Context(), "z", "missing"))
	require.Equal(t, "agent.unavailable", reader(t.Context(), "z", "broken"))
}
