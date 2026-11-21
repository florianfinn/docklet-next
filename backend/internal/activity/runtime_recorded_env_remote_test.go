// Source: docklet-next/docs/design/stack-env-and-peers.md
// Revision: be6efa6e1470df78a90c7d485bc781be1206737a
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Recorded inputs through real Direct and Edge admission and execution; M34 #548.

package activity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestRuntimeRecordedEnvDirectEdge(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			var logs, wire recordedEnvBuffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(previous) })
			restartedEffect := make(chan struct{})
			var restartOnce sync.Once
			f := newRemoteIntegration(t, isEdge, func(agent *runtimeServicesFixture) {
				agent.http.onCall = func(_ http.ResponseWriter, r *http.Request) bool {
					if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/restart") {
						restartOnce.Do(func() { close(restartedEffect) })
					}
					return false
				}
			})
			f.handlerMu.Lock()
			original := f.receiverHandler
			f.receiverHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				original.ServeHTTP(&recordedEnvResponse{ResponseWriter: w, body: &wire}, r)
			})
			f.handlerMu.Unlock()

			dir := f.agent.project("recorded-stack", "recorded-stack", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n    command: [echo, '${VALUE:?required}']\n    environment:\n      VALUE: ${VALUE}\n    labels:\n      example.value: ${VALUE}\n")
			selected := filepath.Join(dir, "custom.yaml")
			require.NoError(t, os.Rename(filepath.Join(dir, "compose.yaml"), selected))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("BROKEN='\n"), 0600))
			env := filepath.Join(f.agent.root, "runtime.env")
			require.NoError(t, os.WriteFile(env, []byte("COMPOSE_FILE=custom.yaml\nVALUE=recorded-sentinel\n"), 0600))
			f.agent.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash", "com.docker.compose.project.working_dir": dir, "com.docker.compose.project.config_files": selected, projects.EnvironmentFileLabel: env})
			f.agent.http.setComposeContainer(true)
			command := f.command(activity.RuntimeRestartProject, activity.RuntimeTarget{Kind: "stack", ID: "recorded-stack"})
			admitted, err := f.hub.Admit(t.Context(), command)
			require.NoError(t, err)
			ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
			_, err = f.control.Start(t.Context(), ref, command.Options)
			require.NoError(t, err)
			wait, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			require.NoError(t, f.control.WaitDispatch(wait, ref.RunID))
			select {
			case <-restartedEffect:
			case <-wait.Done():
				t.Fatalf("agent did not reach the restart effect: %v; events: %s; logs: %s", wait.Err(), f.agent.debugEvents(), logs.String())
			}
			agentResult, err := f.agent.waitLocal(ref.RunID)
			require.NoError(t, err)
			require.Equal(t, activityDTO.StatusSuccess, agentResult.Status, f.agent.debugEvents())
			snapshot, err := f.control.Wait(wait, ref, nil)
			require.NoError(t, err)
			require.Equal(t, activityDTO.StatusSuccess, snapshot.Status)
			effects := runtimeEffectEvents(f.agent.http.snapshot())
			restarted := 0
			for _, effect := range effects {
				if strings.HasSuffix(effect.Path, "/restart") {
					restarted++
				}
			}
			require.Equal(t, 1, restarted, "real project service must execute after fresh recorded-input identity on the agent")
			body, err := json.Marshal([]any{agentResult, snapshot})
			require.NoError(t, err)
			for sink, value := range map[string]string{"hub and executor logs": logs.String(), "agent-proxied bodies": wire.String(), "activity snapshots and output": string(body)} {
				require.NotContains(t, value, "recorded-sentinel", sink)
			}

		})
	}
}

type recordedEnvBuffer struct {
	mu   sync.Mutex
	body bytes.Buffer
}

func (b *recordedEnvBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.body.Write(value)
}
func (b *recordedEnvBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.body.String() }

// Unwrap preserves the real Direct ingress deadline capability while capturing
// the exact bytes forwarded to the hub.
type recordedEnvResponse struct {
	http.ResponseWriter
	body *recordedEnvBuffer
}

func (w *recordedEnvResponse) Write(body []byte) (int, error) {
	n, err := w.ResponseWriter.Write(body)
	_, _ = w.body.Write(body[:n])
	return n, err
}
func (w *recordedEnvResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
