// Source: docklet-next/docs/design/runtime-control.md
// Revision: e70b28ce796c4e118e41048c7114a3a911f613b7e133e15fb0059ed7837dc5b3
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Migrated SQLite and synthetic Docker HTTP coverage of project log labels.

package project

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

type composeLogRequest struct {
	method, path, filters string
}

// Empty inventory ends non-follow logs while retaining the actual SDK's label
// selection. Neither the project/details service nor ComposeLogs is replaced.
func composeLogDocker(t *testing.T) func() []composeLogRequest {
	t.Helper()
	var mu sync.Mutex
	var requests []composeLogRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, composeLogRequest{r.Method, r.URL.Path, r.URL.Query().Get("filters")})
		mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			_, _ = io.WriteString(w, "OK")
		case strings.HasSuffix(r.URL.Path, "/version"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"ApiVersion": "1.41", "MinAPIVersion": "1.24", "Version": "24.0.0"})
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, "[]")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})
	t.Setenv("DOCKER_HOST", dockerHostFromProjectRuntimeServerURL(t, server.URL))
	return func() []composeLogRequest {
		mu.Lock()
		defer mu.Unlock()
		got := append([]composeLogRequest(nil), requests...)
		requests = nil
		return got
	}
}

func assertComposeLogLabel(t *testing.T, svc *ProjectService, id, want string, takeRequests func() []composeLogRequest) {
	t.Helper()
	// Exclude earlier setup/status reads; only the real log call is asserted.
	takeRequests()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	lines := make(chan string)
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for range lines {
		}
	}()
	err := svc.StreamProjectLogs(ctx, id, lines, false, "100", "", false)
	close(lines)
	<-drained
	require.NoError(t, err)
	var lists int
	for _, request := range takeRequests() {
		require.Contains(t, []string{http.MethodGet, http.MethodHead, http.MethodOptions}, request.method, "logs must remain read-only")
		if !strings.HasSuffix(request.path, "/containers/json") {
			require.True(t, strings.HasSuffix(request.path, "/_ping") || strings.HasSuffix(request.path, "/version"), "unexpected SDK request: %s", request.path)
			continue
		}
		lists++
		var filters map[string]map[string]bool
		require.NoError(t, json.Unmarshal([]byte(request.filters), &filters))
		var labels []string
		for label, enabled := range filters["label"] {
			if enabled && strings.HasPrefix(label, "com.docker.compose.project=") {
				labels = append(labels, label)
			}
		}
		t.Logf("log container-list project filters=%q; expected effective label=%q", labels, want)
		require.Equal(t, []string{"com.docker.compose.project=" + want}, labels, "actual log SDK filter must use the effective Compose label without an obsolete/default alias")
	}
	require.Positive(t, lists, "test must traverse ComposeLogs and Docker ContainerList")
}

func assertComposeLogIdentity(t *testing.T, svc *ProjectService, id, stored, effective string, takeRequests func() []composeLogRequest) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	row, err := svc.GetProjectFromDatabaseByID(ctx, id)
	require.NoError(t, err)
	require.Equal(t, id, row.ID)
	require.Equal(t, stored, row.Name, "observations and logs must preserve the stored executable default")
	model, _, err := svc.loadComposeProjectForProject(ctx, row, nil)
	require.NoError(t, err)
	require.Equal(t, effective, model.Name)
	assertComposeLogLabel(t, svc, id, effective, takeRequests)
	after, err := svc.GetProjectFromDatabaseByID(ctx, id)
	require.NoError(t, err)
	require.Equal(t, *row, *after, "streaming logs must not rewrite identity metadata")
}

func TestComposeNameLogsExternalEditAndRefresh(t *testing.T) {
	for _, mode := range []string{"refresh", "sync"} {
		for _, kind := range []string{"literal", "interpolated"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				takeRequests := composeLogDocker(t)
				svc, root := runtimeIdentityFixture(t)
				row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", identityServices)
				for _, effective := range []string{"beta_stack", "next_stack", "alpha"} {
					yaml := identityServices
					if effective != "alpha" {
						name := effective
						if kind == "interpolated" {
							name = "${STACK_NAME}"
						}
						yaml = "name: " + name + "\n" + yaml
					}
					require.NoError(t, os.WriteFile(filepath.Join(row.Path, "compose.yaml"), []byte(yaml), 0600))
					require.NoError(t, os.WriteFile(filepath.Join(row.Path, ".env"), []byte("STACK_NAME="+effective+"\nCOMPOSE_PROJECT_NAME=ignored_env\n"), 0600))
					if mode == "sync" {
						require.NoError(t, svc.SyncProjectsFromFileSystem(t.Context()))
					} else {
						svc.refreshComposeProjectName(t.Context(), row)
					}
					assertComposeLogIdentity(t, svc, row.ID, "alpha", effective, takeRequests)
				}
			})
		}
	}
}

func TestComposeNameLogsInterpolatedCreateAndUpdate(t *testing.T) {
	for _, mode := range []string{"create", "update"} {
		t.Run(mode, func(t *testing.T) {
			takeRequests := composeLogDocker(t)
			svc, _ := runtimeIdentityFixture(t)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			yaml := "name: ${STACK_NAME}\n" + identityServices
			env := "STACK_NAME=beta_stack\n"
			content := yaml
			if mode == "update" {
				content = identityServices
			}
			row, err := svc.CreateProject(ctx, "alpha", content, &env, projecttypes.CreateProjectWorkspaceManifest{}, nil, nil, nil, usertypes.Actor{})
			require.NoError(t, err)
			if mode == "update" {
				_, err = svc.UpdateProject(ctx, row.ID, nil, &yaml, nil, nil, usertypes.Actor{})
				require.NoError(t, err)
			}
			assertComposeLogIdentity(t, svc, row.ID, "alpha", "beta_stack", takeRequests)
			for _, effective := range []string{"next_stack", "alpha", "restored_stack"} {
				content = yaml
				if effective == "alpha" {
					content = identityServices
				}
				updated, err := svc.UpdateProject(ctx, row.ID, nil, &content, new("STACK_NAME="+effective+"\n"), nil, usertypes.Actor{})
				require.NoError(t, err)
				require.Equal(t, row.ID, updated.ID)
				assertComposeLogIdentity(t, svc, row.ID, "alpha", effective, takeRequests)
			}
		})
	}
}

func TestComposeNameLogsLegacyFallback(t *testing.T) {
	for _, metadata := range []*string{nil, new("")} {
		name := "nil-metadata"
		if metadata != nil {
			name = "empty-metadata"
		}
		t.Run(name, func(t *testing.T) {
			takeRequests := composeLogDocker(t)
			svc, root := runtimeIdentityFixture(t)
			row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", identityServices)
			row.Name, row.ComposeProjectName = "Legacy Custom!", metadata
			require.NoError(t, svc.db.Save(row).Error)
			assertComposeLogIdentity(t, svc, row.ID, "Legacy Custom!", "legacycustom", takeRequests)
		})
	}
}

func TestComposeNameLogsUsesRecordedObservation(t *testing.T) {
	takeRequests := composeLogDocker(t)
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", "name: beta_stack\n"+identityServices)
	svc.refreshComposeProjectName(t.Context(), row)
	require.Equal(t, new("beta_stack"), row.ComposeProjectName)
	// Legacy logs remain available from recorded metadata even when current
	// files cannot load. Refresh/admission retain their separate failure policy.
	require.NoError(t, os.WriteFile(filepath.Join(row.Path, ".env"), []byte("BROKEN='unterminated\n"), 0600))
	_, _, err := svc.loadComposeProjectForProject(t.Context(), row, nil)
	require.Error(t, err)
	assertComposeLogLabel(t, svc, row.ID, "beta_stack", takeRequests)
	require.NoError(t, os.RemoveAll(row.Path))
	assertComposeLogLabel(t, svc, row.ID, "beta_stack", takeRequests)
}
