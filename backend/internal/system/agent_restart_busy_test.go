// Source: docklet-next/issues/368
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original busy refusal proofs for the agent self restart receiver.

package system

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

// Running local backup or restore work refuses the restart with 409 host.busy
// before the 202, so the Docker restart never runs; idle work still restarts.
func TestAgentRestartRefusedWhileBackupWorkRuns(t *testing.T) {
	for _, scenario := range []string{"busy", "unreadable", "idle"} {
		t.Run(scenario, func(t *testing.T) {
			var restartCalls atomic.Int32
			dockerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					restartCalls.Add(1)
					w.WriteHeader(204)
					return
				}
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"Id": "inspected-self"}))
			}))
			defer dockerServer.Close()
			dockerClient, err := client.New(client.WithHost(dockerServer.URL), client.WithAPIVersion("1.54"))
			require.NoError(t, err)
			defer dockerClient.Close()
			var probes atomic.Int32
			h := &agentRestartHandler{appCtx: t.Context(), selfID: func() (string, error) { return "self", nil },
				client: func(context.Context) (*client.Client, error) { return dockerClient, nil },
				wait:   func(context.Context, time.Duration) bool { return true },
				busy: func(context.Context) (bool, error) {
					probes.Add(1)
					switch scenario {
					case "busy":
						return true, nil
					case "unreadable":
						return false, errors.New("backup store unavailable")
					}
					return false, nil
				}}
			rec := httptest.NewRecorder()
			restartTestMux(h).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/system/agent/restart", http.NoBody))
			require.EqualValues(t, 1, probes.Load(), "the busy check runs once per request")
			if scenario == "idle" {
				require.Equal(t, 202, rec.Code, rec.Body.String())
				require.Eventually(t, func() bool { return restartCalls.Load() == 1 }, 10*time.Second, 10*time.Millisecond)
				return
			}
			require.Equal(t, 409, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "host.busy")
			require.Zero(t, restartCalls.Load(), "a refused restart never reaches Docker")
		})
	}
}
