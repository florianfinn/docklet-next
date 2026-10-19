// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Actual SQLite list/detail runtime JSON bound to a sanitized web fixture.

package project

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/project/children/listing"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

// Compare only the shared fields, retaining real service serialization rather
// than replacing runtime membership with a hand-built project DTO.
func assertRuntimeMembershipWire(t *testing.T, response projecttypes.Details) {
	t.Helper()
	fixture, err := os.ReadFile("../../../web/src/data/http/project-runtime.test-support.ts")
	require.NoError(t, err)
	_, literal, found := strings.Cut(string(fixture), "export default ")
	require.True(t, found)
	var expected, wire map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimSpace(literal), ";")), &expected))
	data, err := json.Marshal(response)
	require.NoError(t, err)
	t.Logf("actual runtime JSON: %s", data)
	require.NoError(t, json.Unmarshal(data, &wire))
	for key, value := range expected {
		if key == "runtimeServices" {
			services, ok := wire[key].([]any)
			require.True(t, ok, "actual wire must carry explicit runtime membership")
			require.Len(t, services, 1)
			for field, want := range value.([]any)[0].(map[string]any) {
				require.Equal(t, want, services[0].(map[string]any)[field], "runtime field %s", field)
			}
			continue
		}
		require.Equal(t, value, wire[key], "actual response field %s", key)
	}
}

func TestComposeNameRuntimeMembershipWire(t *testing.T) {
	for _, mode := range []string{"manual-label", "external-literal", "external-env"} {
		t.Run(mode, func(t *testing.T) {
			svc, root := runtimeIdentityFixture(t)
			compose := identityServices
			if mode == "external-env" {
				compose = "name: ${STACK_NAME:-alpha}\n" + compose
			}
			row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", compose)
			svc.refreshComposeProjectName(t.Context(), row)
			if mode == "external-literal" {
				require.NoError(t, os.WriteFile(filepath.Join(row.Path, "compose.yaml"), []byte("name: manual_p\n"+identityServices), 0600))
			} else if mode == "external-env" {
				require.NoError(t, os.WriteFile(filepath.Join(row.Path, ".env"), []byte("STACK_NAME=manual_p\n"), 0600))
			}
			// Metadata remains alpha until the next watcher/refresh. The manual
			// case keeps the default model; the others now execute as manual_p.
			before, err := svc.GetProjectFromDatabaseByID(t.Context(), row.ID)
			require.NoError(t, err)
			require.Equal(t, "alpha", observedComposeName(*before))
			labels := map[string]string{
				"com.docker.compose.project": "manual_p", "com.docker.compose.service": "app",
				"com.docker.compose.project.working_dir": row.Path,
				"com.docker.compose.config-hash":         "example-hash",
			}
			containers := []container.Summary{{ID: "c-fallback", Names: []string{"/manual_p-app-1"},
				State: "running", Status: "Up", Image: "registry.example.com/app:1", Labels: labels}}
			svc.listing = listing.New(func(context.Context) ([]container.Summary, error) { return containers, nil }, nil, nil)
			var mutations, unexpected, scopedLists atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet && !(r.Method == http.MethodHead && strings.HasSuffix(r.URL.Path, "/_ping")) {
					mutations.Add(1)
					http.Error(w, "read-only fixture", http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/_ping"):
					_, _ = w.Write([]byte("OK"))
				case strings.HasSuffix(r.URL.Path, "/version"):
					_ = json.NewEncoder(w).Encode(map[string]string{"ApiVersion": "1.41", "MinAPIVersion": "1.24", "Version": "24.0.0"})
				case strings.HasSuffix(r.URL.Path, "/containers/json"):
					if strings.Contains(r.URL.Query().Get("filters"), "com.docker.compose.project=manual_p") {
						scopedLists.Add(1)
					}
					_ = json.NewEncoder(w).Encode(containers)
				case strings.HasSuffix(r.URL.Path, "/containers/c-fallback/json"):
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": "c-fallback", "State": map[string]any{"Status": "running"}})
				default:
					unexpected.Add(1)
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(func() { server.CloseClientConnections(); server.Close() })
			t.Setenv("DOCKER_HOST", dockerHostFromProjectRuntimeServerURL(t, server.URL))
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			for _, params := range []pagination.QueryParams{{Limit: -1}, {Limit: -1, Sort: "status"}} {
				rows, _, err := svc.ListProjects(ctx, params)
				require.NoError(t, err)
				require.Len(t, rows, 1)
				assertRuntimeMembershipWire(t, rows[0])
			}
			if mode != "manual-label" {
				// Details uses the freshly loaded model and real SDK Ps, whereas
				// listing reaches the retained unique-directory fallback above.
				detail, err := svc.GetProjectDetails(ctx, row.ID, projecttypes.DetailsOptions{IncludeRuntimeServices: true})
				require.NoError(t, err)
				assertRuntimeMembershipWire(t, detail)
				require.Positive(t, scopedLists.Load(), "actual SDK must list the current model label")
			}
			require.Zero(t, mutations.Load())
			require.Zero(t, unexpected.Load())
			after, err := svc.GetProjectFromDatabaseByID(ctx, row.ID)
			require.NoError(t, err)
			require.Equal(t, before.ID, after.ID)
			require.Equal(t, before.Name, after.Name)
			require.Equal(t, before.Path, after.Path)
			require.Equal(t, before.ComposeProjectName, after.ComposeProjectName, "read projections must not refresh the stale observation")
		})
	}
}
