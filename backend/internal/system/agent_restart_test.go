// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original self restart receiver HTTP and Docker seam tests.

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

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/florianfinn/docklet-next/backend/internal/health"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/environment"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

func restartTestMux(h *agentRestartHandler) http.Handler {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Restart", "0.0.0"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	h.register(api)
	return mux
}

func TestAgentRestartResolvesAndInspectsBefore202(t *testing.T) {
	for _, scenario := range []string{"supported", "no-self", "resolve-error", "docker-unavailable", "inspect-error", "inspect-empty"} {
		t.Run(scenario, func(t *testing.T) {
			var inspectCalls, restartCalls atomic.Int32
			restarted := make(chan string, 1)
			dockerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case "GET":
					inspectCalls.Add(1)
					require.Contains(t, r.URL.Path, "/containers/self/json")
					if scenario == "inspect-error" {
						w.WriteHeader(500)
						return
					}
					id := "inspected-self"
					if scenario == "inspect-empty" {
						id = ""
					}
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"Id": id}))
				case "POST":
					restartCalls.Add(1)
					restarted <- r.URL.Path
					w.WriteHeader(204)
				default:
					t.Errorf("unexpected Docker request %s", r.Method)
					w.WriteHeader(500)
				}
			}))
			defer dockerServer.Close()
			dockerClient, err := client.New(client.WithHost(dockerServer.URL), client.WithAPIVersion("1.54"))
			require.NoError(t, err)
			defer dockerClient.Close()
			entered, release := make(chan time.Duration, 1), make(chan struct{})
			h := &agentRestartHandler{appCtx: t.Context(), selfID: func() (string, error) { return "self", nil }, client: func(context.Context) (*client.Client, error) { return dockerClient, nil }, wait: func(ctx context.Context, d time.Duration) bool {
				entered <- d
				select {
				case <-release:
					return true
				case <-ctx.Done():
					return false
				}
			}}
			switch scenario {
			case "no-self":
				h.selfID = func() (string, error) { return "", nil }
			case "resolve-error":
				h.selfID = func() (string, error) { return "", errors.New("not in Docker") }
			case "docker-unavailable":
				h.client = func(context.Context) (*client.Client, error) { return nil, errors.New("unavailable") }
			}
			rec := httptest.NewRecorder()
			restartTestMux(h).ServeHTTP(rec, httptest.NewRequest("POST", "/system/agent/restart", http.NoBody))
			if scenario != "supported" {
				require.Equal(t, 409, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), "agent.restart-unsupported")
				require.Zero(t, restartCalls.Load())
				if scenario == "no-self" || scenario == "resolve-error" || scenario == "docker-unavailable" {
					require.Zero(t, inspectCalls.Load())
				}
				return
			}
			require.Equal(t, 202, rec.Code, rec.Body.String())
			var out dto.AgentRestartResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			require.Equal(t, health.InstanceID(), out.InstanceID)
			select {
			case delay := <-entered:
				require.Equal(t, time.Second, delay)
			case <-time.After(5 * time.Second):
				t.Fatal("restart worker did not reach delay barrier")
			}
			require.EqualValues(t, 1, inspectCalls.Load())
			require.Zero(t, restartCalls.Load(), "202 must precede Docker restart")
			close(release)
			select {
			case path := <-restarted:
				require.Contains(t, path, "/containers/inspected-self/restart")
			case <-time.After(5 * time.Second):
				t.Fatal("Docker restart not observed")
			}
			require.EqualValues(t, 1, restartCalls.Load())
		})
	}
}
