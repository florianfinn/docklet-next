// Source: docklet-next/issues/465
// Revision: b4f894ff7f35186d4c39be1fc9fccd01b4b5934a
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original allowlisted blocker detail over real Direct and Edge runtime transport.

package activity_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	a "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
)

func TestRuntimeBlockerDetailDirectEdge(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		name := "Direct"
		if isEdge {
			name = "Edge"
		}
		t.Run(name, func(t *testing.T) {
			f := newRemoteIntegration(t, isEdge)
			f.agent.project("example-target", "example-target", "name: example_target\nservices:\n  app:\n    image: registry.example.com/app:1\n")
			path := f.agent.project("example-peer", "example-peer", "services:\n  app:\n    image: registry.example.com/app:1\n")
			require.NoError(t, os.Mkdir(filepath.Join(path, "deploy"), 0700))
			require.NoError(t, os.WriteFile(filepath.Join(path, "deploy", "compose.yaml"), []byte("services:\n  app:\n    image: registry.example.com/app:1\n"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(path, ".env"), []byte("COMPOSE_FILE=deploy/compose.yaml\n"), 0600))
			require.NoError(t, f.agent.db.Model(&project.Project{}).Where("id = ?", "example-peer").Update("is_archived", true).Error)
			request := a.RuntimeTargetRequest{Registration: a.RuntimeExecutorIdentity{ManagerID: f.store.ManagerID, RegistrationID: f.store.RegistrationID, ExecutorGeneration: f.store.ExecutorGeneration}, Target: activity.RuntimeTarget{Kind: "stack", ID: "example-target"}}
			_, err := f.sender.Resolve(t.Context(), request)
			var problem *imageDTO.OperationError
			require.ErrorAs(t, err, &problem)
			require.Equal(t, "runtime.target-unavailable", problem.Problem.Code)
			require.True(t, problem.Problem.Blocker.Confirmable)
			require.Equal(t, "compose.selection-unsupported", problem.Problem.Blocker.Uncertainty.Peers[0].Reason)
			require.True(t, problem.Problem.Blocker.Uncertainty.Peers[0].Archived)
			encoded, err := json.Marshal(problem)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), path)
			require.Zero(t, f.executeCalls.Load(), "identity refusal must precede dispatch")

			// Authenticated transport does not authorize arbitrary diagnostics or blocker fields.
			for _, candidate := range []struct{ code, name, reason string }{
				{"runtime.target-unavailable", "/srv/private", "compose.selection-unsupported"},
				{"runtime.target-unavailable", "example-peer", "arbitrary.diagnostic"},
				{"runtime.target-unavailable", "example\npeer", "compose.identity-unreadable"},
				{"image.not-found", "example-peer", "compose.selection-unsupported"},
			} {
				f.handlerMu.Lock()
				f.receiverHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body := imageDTO.Failure(candidate.code, 409)
					body.Problem.Message = "private diagnostic"
					body.Problem.Blocker = &imageDTO.RuntimeIdentityBlocker{Name: candidate.name, Reason: candidate.reason}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(409)
					require.NoError(t, json.NewEncoder(w).Encode(body))
				})
				f.handlerMu.Unlock()
				_, err = f.sender.Resolve(t.Context(), request)
				require.ErrorAs(t, err, &problem)
				require.Nil(t, problem.Problem.Blocker)
				require.NotContains(t, problem.Problem.Message, "private diagnostic")
			}
		})
	}
}
