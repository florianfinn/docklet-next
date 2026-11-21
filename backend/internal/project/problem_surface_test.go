// Source: docklet-next/docs/design/stack-env-and-peers.md
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original per-stack load problem and Compose name peer tests for issue 547.

package project

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	dto "github.com/florianfinn/docklet-next/types/project"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

const problemSentinel = "example-sentinel-value"

func listedRows(t *testing.T, svc *ProjectService, containers []container.Summary) map[string]dto.Details {
	t.Helper()
	server := newProjectRuntimeDockerServer(t, containers)
	svc.dockerService = &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	svc.initChildren(nil, nil)
	rows, _, err := svc.ListProjects(t.Context(), pagination.QueryParams{Limit: -1})
	require.NoError(t, err)
	out := map[string]dto.Details{}
	for _, row := range rows {
		out[row.ID] = row
	}
	return out
}

func TestRunningUnreadableStackNamesItsMissingVariable(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	edge := runtimeIdentityProject(t, svc, root, "example-edge", "edge-stack", "name: edge\nservices:\n  app:\n    image: registry.example.com/app:${APP_TAG:?"+problemSentinel+"}\n")
	healthy := runtimeIdentityProject(t, svc, root, "example-healthy", "healthy", "name: healthy\n"+identityServices)
	running := []container.Summary{{ID: "example-container", Names: []string{"/edge-app-1"}, Image: "registry.example.com/app:1", State: "running", Labels: map[string]string{"com.docker.compose.project": "edge", "com.docker.compose.service": "app", "com.docker.compose.project.working_dir": edge.Path}}}
	rows := listedRows(t, svc, running)
	row := rows[edge.ID]
	require.NotEmpty(t, row.RuntimeServices, "precondition: the stack has running containers")
	require.Equal(t, "definition-invalid", row.DefinitionHint, "a running stack shows its own problem")
	require.Equal(t, "compose.variable-missing", row.DefinitionReason)
	require.Equal(t, "APP_TAG", row.DefinitionVariable)
	require.Empty(t, rows[healthy.ID].DefinitionHint, "other stacks are unaffected")
	body, err := json.Marshal(rows)
	require.NoError(t, err)
	require.NotContains(t, string(body), problemSentinel, "the operator message never reaches the API")
}

func TestComposeNamePeersWarnAcrossReadableUnreadableAndArchived(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	a := runtimeIdentityProject(t, svc, root, "example-a", "alpha", "name: shared\n"+identityServices)
	b := runtimeIdentityProject(t, svc, root, "example-b", "beta", "name: shared\nservices:\n  app:\n    image: registry.example.com/app:${TAG:?required}\n")
	c := runtimeIdentityProject(t, svc, root, "example-c", "gamma", "name: other\n"+identityServices)
	d := runtimeIdentityProject(t, svc, root, "example-d", "delta", "name: ${NAME:?required}\n"+identityServices)
	require.NoError(t, svc.db.Model(b).Update("is_archived", true).Error)
	rows := listedRows(t, svc, nil)
	require.Equal(t, []string{"beta"}, rows[a.ID].ComposeNamePeers, "an archived, unreadable peer with a literal name counts")
	require.Empty(t, rows[c.ID].ComposeNamePeers)
	require.Empty(t, rows[d.ID].ComposeNamePeers, "no evidence, no warning")
	require.NoError(t, os.WriteFile(filepath.Join(c.Path, "compose.yaml"), []byte("name: shared\n"+identityServices), 0600))
	rows = listedRows(t, svc, nil)
	require.ElementsMatch(t, []string{"beta", "gamma"}, rows[a.ID].ComposeNamePeers)
	require.ElementsMatch(t, []string{"alpha", "beta"}, rows[c.ID].ComposeNamePeers)
}

func TestLoadProblemsNeverLogOrReturnRawText(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	svc, root := runtimeIdentityFixture(t)
	edge := runtimeIdentityProject(t, svc, root, "example-edge", "edge-stack", "name: edge\nservices:\n  app:\n    image: registry.example.com/app:${APP_TAG:?"+problemSentinel+"}\n")
	broken := runtimeIdentityProject(t, svc, root, "example-broken", "broken", "name: broken\nservices:\n  app:\n    image: registry.example.com/app:${TAG}\n")
	require.NoError(t, os.WriteFile(filepath.Join(broken.Path, ".env"), []byte("TAG=\""+problemSentinel+"\nOTHER=1\n"), 0600))
	rows := listedRows(t, svc, nil)
	_, servicesErr := svc.projectServices(t.Context(), edge.ID)
	_, _, identityErr := svc.ResolveRuntimeProjectIdentity(t.Context(), edge.ID, "")
	_, _, brokenErr := svc.ResolveRuntimeProjectIdentity(t.Context(), broken.ID, "")
	body, err := json.Marshal(rows)
	require.NoError(t, err)
	require.Error(t, servicesErr)
	require.Error(t, identityErr)
	require.Error(t, brokenErr)
	for name, text := range map[string]string{"logs": logs.String(), "list": string(body), "services": servicesErr.Error()} {
		require.NotContains(t, text, problemSentinel, name)
	}
	require.Contains(t, logs.String(), "compose.variable-missing (APP_TAG)", "precondition: the sink logged the classified problem")
}

func TestComposeNamePeersHiddenForRestrictedCallers(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	own := runtimeIdentityProject(t, svc, root, "example-own", "own", "name: shared\n"+identityServices)
	runtimeIdentityProject(t, svc, root, "example-foreign", "foreign-private-name", "name: shared\n"+identityServices)
	require.Equal(t, []string{"foreign-private-name"}, listedRows(t, svc, nil)[own.ID].ComposeNamePeers, "precondition: administrators see the warning")
	rows, _, err := svc.ListProjects(projects.WithPeerNamesHidden(t.Context()), pagination.QueryParams{Limit: -1})
	require.NoError(t, err)
	detail, err := svc.GetProjectDetails(projects.WithPeerNamesHidden(t.Context()), own.ID, dto.DetailsOptions{})
	require.NoError(t, err)
	for _, row := range rows {
		require.Empty(t, row.ComposeNamePeers, row.ID)
	}
	body, err := json.Marshal(detail)
	require.NoError(t, err)
	require.NotContains(t, string(body), "foreign-private-name")
}

func TestComposeNamePeersNeedUnambiguousLiteralEvidence(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	healthy := runtimeIdentityProject(t, svc, root, "example-healthy", "healthy", "name: shared\n"+identityServices)
	aux := runtimeIdentityProject(t, svc, root, "example-aux", "aux", "name: shared\nservices:\n  app:\n    image: registry.example.com/app:${TAG:?required}\n")
	require.NoError(t, os.WriteFile(filepath.Join(aux.Path, ".env"), []byte("COMPOSE_ENV_FILES=runtime.env\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(aux.Path, "runtime.env"), []byte("COMPOSE_PROJECT_NAME=distinct\n"), 0600))
	recorded := runtimeIdentityProject(t, svc, root, "example-recorded", "recorded", "name: shared\nservices:\n  app:\n    image: registry.example.com/app:${TAG:?required}\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, "recorded.env"), []byte("COMPOSE_PROJECT_NAME=other\n"), 0600))
	labels := map[string]string{"com.docker.compose.project": "other", "com.docker.compose.service": "app", "com.docker.compose.project.working_dir": recorded.Path, "com.docker.compose.project.config_files": filepath.Join(recorded.Path, "compose.yaml"), projects.EnvironmentFileLabel: filepath.Join(root, "recorded.env")}
	rows := listedRows(t, svc, []container.Summary{{ID: "example-recorded-container", Names: []string{"/other-app-1"}, Image: "registry.example.com/app:1", State: "exited", Labels: labels}})
	require.Empty(t, rows[healthy.ID].ComposeNamePeers, "auxiliary or recorded inputs may select another name: no evidence")
}
