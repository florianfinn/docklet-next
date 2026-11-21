// Source: docklet-next/issues/465
// Revision: b4f894ff7f35186d4c39be1fc9fccd01b4b5934a
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: M34 #547: unreadable peers never block; duplicates block name-addressed lookups. Original typed fail-closed peer identity proof.

package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRuntimeIdentityNamesBlockingPeer(t *testing.T) {
	for _, archived := range []bool{false, true} {
		for _, lookup := range []string{"stack", "container"} {
			t.Run(lookup+"/archived="+strconv.FormatBool(archived), func(t *testing.T) {
				f := newLifecycleFixture(t)
				peerPath := filepath.Join(f.root, "example-peer")
				require.NoError(t, os.MkdirAll(filepath.Join(peerPath, "deploy"), 0700))
				require.NoError(t, os.WriteFile(filepath.Join(peerPath, "deploy", "compose.yaml"), []byte("services:\n  app:\n    image: registry.example.com/app:1\n"), 0600))
				require.NoError(t, os.WriteFile(filepath.Join(peerPath, ".env"), []byte("COMPOSE_FILE=deploy/compose.yaml\n"), 0600))
				peer := &Project{BaseModel: database.BaseModel{ID: uuid.NewString()}, Name: "example-peer", Path: peerPath, DirName: new("example-peer"), IsArchived: archived}
				require.NoError(t, f.db.Create(peer).Error)
				id, name := f.project.ID, ""
				if lookup == "container" {
					id, name = "", "example-stack"
				}
				_, found, err := f.service.ResolveRuntimeProjectIdentity(t.Context(), id, name)
				require.NoError(t, err, "an unreadable peer never blocks")
				require.True(t, found)

				// A readable peer with the same name blocks only the name-addressed lookup.
				require.NoError(t, os.WriteFile(filepath.Join(peerPath, ".env"), nil, 0600))
				require.NoError(t, os.WriteFile(filepath.Join(peerPath, "compose.yaml"), []byte("name: example-stack\nservices:\n  app:\n    image: registry.example.com/app:1\n"), 0600))
				_, found, err = f.service.ResolveRuntimeProjectIdentity(t.Context(), id, name)
				if lookup == "stack" {
					require.NoError(t, err, "a duplicate name is a warning for a project-ID action")
					require.True(t, found)
					return
				}
				require.False(t, found)
				var problem *imageDTO.OperationError
				require.ErrorAs(t, err, &problem)
				require.Equal(t, "runtime.target-unavailable", problem.Problem.Code)
				encoded, err := json.Marshal(problem)
				require.NoError(t, err)
				var body struct {
					Error struct {
						Blocker struct {
							Name        string
							Reason      string
							Archived    bool
							Confirmable bool
						}
					}
				}
				require.NoError(t, json.Unmarshal(encoded, &body))
				require.Equal(t, "compose.duplicate", body.Error.Blocker.Reason)
				require.False(t, body.Error.Blocker.Confirmable)
				require.Contains(t, []string{peer.Name, f.project.Name}, body.Error.Blocker.Name)
				require.NotContains(t, string(encoded), f.root)
			})
		}
	}
}
