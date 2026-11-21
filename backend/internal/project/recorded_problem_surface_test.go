// Source: docklet-next/docs/design/stack-env-and-peers.md
// Revision: be6efa6e1470df78a90c7d485bc781be1206737a
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Recorded-file problems and API/log leak regressions; M34 #548.

package project

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

func TestRecordedProblemsStayTargetLocalAndSafe(t *testing.T) {
	for _, kind := range []string{"invalid", "conflict", "required", "dotenv"} {
		t.Run(kind, func(t *testing.T) {
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(previous) })
			svc, root := runtimeIdentityFixture(t)
			row := runtimeIdentityProject(t, svc, root, "recorded", "recorded", "name: recorded\nservices:\n  app:\n    image: alpine:3.22\n    environment:\n      VALUE: ${VALUE}\n    command: [echo, '${MISSING:?recorded-sentinel}']\n    labels:\n      example.value: ${VALUE}\n")
			healthy := runtimeIdentityProject(t, svc, root, "healthy", "healthy", "name: healthy\n"+identityServices)
			path := filepath.Join(root, "runtime.env")
			require.NoError(t, os.WriteFile(path, []byte("VALUE=recorded-sentinel\n"), 0600))
			label := map[string]string{"com.docker.compose.project": "recorded", "com.docker.compose.service": "app", "com.docker.compose.project.working_dir": row.Path, "com.docker.compose.project.config_files": filepath.Join(row.Path, "compose.yaml"), projects.EnvironmentFileLabel: path}
			summaries := []container.Summary{{ID: "example-container", Image: "alpine:3.22", State: container.StateRunning, Labels: label}}
			reason := "compose.variable-missing"
			switch kind {
			case "invalid":
				label[projects.EnvironmentFileLabel] = "/srv/example/outside.env"
				reason = "compose.env-label-invalid"
			case "conflict":
				second := map[string]string{}
				for k, v := range label {
					second[k] = v
				}
				second[projects.EnvironmentFileLabel] = ""
				summaries = append(summaries, container.Summary{ID: "example-other-container", Labels: second})
				reason = "compose.env-label-conflict"
			case "dotenv":
				require.NoError(t, os.WriteFile(path, []byte("VALUE=\"recorded-sentinel\n"), 0600))
				reason = "compose.env-label-invalid"
			}
			server := newProjectRuntimeDockerServer(t, summaries)
			t.Setenv("DOCKER_HOST", dockerHostFromProjectRuntimeServerURL(t, server.URL))
			svc.dockerService = &docker.DockerClientService{Client: newTestDockerClient(t, server)}
			svc.initChildren(nil, nil)
			rows, _, err := svc.ListProjects(t.Context(), pagination.QueryParams{Limit: -1})
			require.NoError(t, err)
			require.Len(t, rows, 2)
			for _, item := range rows {
				if item.ID == row.ID {
					require.Equal(t, reason, item.DefinitionReason)
				} else {
					require.Empty(t, item.DefinitionReason)
				}
			}
			detail, err := svc.GetProjectDetails(t.Context(), row.ID, projectDTO.DetailsOptions{IncludeServiceConfigs: true, IncludeRuntimeServices: true})
			require.NoError(t, err)
			require.Equal(t, reason, detail.DefinitionReason)
			_, _, loadErr := svc.loadComposeProjectForProject(t.Context(), row, nil)
			require.Error(t, loadErr)
			_, _, identityErr := svc.ResolveRuntimeProjectIdentity(t.Context(), row.ID, "")
			require.Error(t, identityErr)
			_, _, healthyErr := svc.ResolveRuntimeProjectIdentity(t.Context(), healthy.ID, "")
			require.NoError(t, healthyErr)
			for name, value := range map[string]any{"list": rows, "details": detail, "load": loadErr.Error(), "identity": identityErr.Error(), "logs": logs.String()} {
				body, err := json.Marshal(value)
				require.NoError(t, err)
				require.NotContains(t, string(body), "recorded-sentinel", name)
			}
		})
	}
}

func TestRecordedAPIServicesKeepPlaceholders(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "recorded", "recorded", "name: recorded\nservices:\n  app:\n    image: alpine:3.22\n    environment:\n      VALUE: ${VALUE}\n    command: [echo, '${VALUE}']\n    labels:\n      example.value: ${VALUE}\n")
	path := filepath.Join(root, "runtime.env")
	require.NoError(t, os.WriteFile(path, []byte("VALUE=recorded-sentinel\n"), 0600))
	label := map[string]string{"com.docker.compose.project": "recorded", "com.docker.compose.service": "app", "com.docker.compose.project.working_dir": row.Path, "com.docker.compose.project.config_files": filepath.Join(row.Path, "compose.yaml"), projects.EnvironmentFileLabel: path}
	ctx := projects.WithComposeContainerLabels(t.Context(), func(context.Context) ([]map[string]string, error) { return []map[string]string{label}, nil })
	model, composePath, err := svc.loadComposeProjectForProject(ctx, row, nil)
	require.NoError(t, err)
	// ComposeServices folds both runtime and not-created service projections. With
	// no Docker containers the latter still traverses the actual DTO producer.
	server := newProjectRuntimeDockerServer(t, nil)
	t.Setenv("DOCKER_HOST", dockerHostFromProjectRuntimeServerURL(t, server.URL))
	svc.dockerService = &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	svc.initChildren(nil, nil)
	services, err := svc.details.ComposeServices(ctx, model, composePath, root, false, "selfhst")
	require.NoError(t, err)
	require.Len(t, services, 1)
	require.NotNil(t, services[0].ServiceConfig)
	require.Equal(t, "${VALUE}", *services[0].ServiceConfig.Environment["VALUE"])
	body, err := json.Marshal(services)
	require.NoError(t, err)
	require.NotContains(t, string(body), "recorded-sentinel")
}

func TestRecordedAttributionListsOncePerRequest(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	a := runtimeIdentityProject(t, svc, root, "example-a", "alpha", "name: alpha\n"+identityServices)
	b := runtimeIdentityProject(t, svc, root, "example-b", "beta", "name: beta\n"+identityServices)
	path := filepath.Join(root, "runtime.env")
	require.NoError(t, os.WriteFile(path, []byte("VALUE=synthetic\n"), 0600))
	summaries := []container.Summary{}
	for _, row := range []*Project{a, b} {
		summaries = append(summaries, container.Summary{ID: row.ID + "-container", Image: "alpine:3.22", State: container.StateRunning, Labels: map[string]string{"com.docker.compose.project": row.Name, "com.docker.compose.service": "app", "com.docker.compose.project.working_dir": row.Path, "com.docker.compose.project.config_files": filepath.Join(row.Path, "compose.yaml"), projects.EnvironmentFileLabel: path}})
	}
	base := newProjectRuntimeDockerServer(t, summaries)
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/containers/json") && strings.Contains(r.URL.Query().Get("filters"), "com.docker.compose.project.working_dir") {
			reads.Add(1)
		}
		base.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	svc.dockerService = &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	svc.initChildren(nil, nil)
	for request := 1; request <= 2; request++ {
		rows, _, err := svc.ListProjects(t.Context(), pagination.QueryParams{Limit: -1})
		require.NoError(t, err)
		require.Len(t, rows, 2)
		require.EqualValues(t, request, reads.Load(), "all attribution in each request shares one fresh listing")
	}
}
