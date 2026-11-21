// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531. M34 #547: unreadable peers are skipped; duplicates refuse only name-addressed lookups.

package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/project"
	"github.com/stretchr/testify/require"
)

const peerService = "services:\n  app:\n    image: registry.example.com/app:1\n    env_file: [service.env]\n"

func s4Problem(t *testing.T, err error) *imageDTO.RuntimeIdentityBlocker {
	t.Helper()
	var problem *imageDTO.OperationError
	require.ErrorAs(t, err, &problem)
	require.Equal(t, "runtime.target-unavailable", problem.Problem.Code)
	require.NotNil(t, problem.Problem.Blocker)
	return problem.Problem.Blocker
}

func TestS4DegradedEvidenceAndDuplicatePrecedence(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	target := runtimeIdentityProject(t, svc, root, "example-target", "target", "name: target_stack\n"+identityServices)
	peer := runtimeIdentityProject(t, svc, root, "example-peer", "peer", "name: peer_stack\n"+peerService)
	for _, archived := range []bool{false, true} {
		require.NoError(t, svc.db.Model(peer).Update("is_archived", archived).Error)
		ref, found, used, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", "")
		require.NoError(t, err, "a proven distinct degraded peer passes automatically")
		require.True(t, found)
		require.Equal(t, target.ID, ref.ID)
		require.Nil(t, used)
	}
	unknown := runtimeIdentityProject(t, svc, root, "example-unknown", "unknown", "name: ${PEER_NAME:?required}\n"+identityServices)
	require.NoError(t, os.WriteFile(filepath.Join(peer.Path, "compose.yaml"), []byte("name: target_stack\n"+peerService), 0600))
	// A project-ID action proceeds despite the duplicate; the name must stay unambiguous.
	ref, found, used, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", "")
	require.NoError(t, err, "a duplicate name is a warning for a project-ID action")
	require.True(t, found)
	require.Equal(t, target.ID, ref.ID)
	require.Nil(t, used)
	for _, selector := range []struct{ id, name string }{{"", "target_stack"}, {target.ID, "target_stack"}} {
		_, found, _, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), selector.id, selector.name, "")
		b := s4Problem(t, err)
		require.False(t, found)
		require.False(t, b.Confirmable)
		require.Equal(t, "compose.duplicate", b.Reason, "a name-addressed lookup needs one owner")
	}
	require.NoError(t, svc.db.Delete(unknown).Error)
	// A missing service environment remains a non-overridable target failure.
	_, found, _, err = svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), peer.ID, "", "")
	require.Error(t, err)
	require.False(t, found)
}

func TestS4UnreadablePeersNeverBlockAnotherStack(t *testing.T) {
	for _, tc := range []struct{ name, compose, env, extra string }{
		{"interpolation", "name: ${PEER_NAME:?required}\n" + identityServices, "", ""},
		{"literal-name-missing-variable", "name: edge\nservices:\n  app:\n    image: registry.example.com/app:${APP_TAG:?set in an env file}\n", "", ""},
		{"yaml", "not: [valid", "", ""},
		{"primary-env", identityServices, "VALUE=\"unterminated\n", ""},
		{"selection", identityServices, "COMPOSE_FILE=nested/compose.yaml\n", "nested/compose.yaml"},
		{"mixed-env", "name: peer_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n    env_file: [service.env, invalid.env]\n", "", "invalid.env"},
		{"label-file", "name: peer_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n    label_file: labels.env\n", "", ""},
		{"include", "include: [missing.yaml]\n" + identityServices, "", ""},
		{"permission", "name: peer_stack\n" + peerService, "", "service.env"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, root := runtimeIdentityFixture(t)
			target := runtimeIdentityProject(t, svc, root, "example-target", "target", identityServices)
			peer := runtimeIdentityProject(t, svc, root, "example-peer", "peer", tc.compose)
			if tc.env != "" {
				require.NoError(t, os.WriteFile(filepath.Join(peer.Path, ".env"), []byte(tc.env), 0600))
			}
			if tc.extra != "" {
				path := filepath.Join(peer.Path, tc.extra)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
				body := identityServices
				if tc.name == "mixed-env" {
					body = "VALUE=\"unterminated\n"
				}
				require.NoError(t, os.WriteFile(path, []byte(body), 0600))
				if tc.name == "permission" {
					require.NoError(t, os.Chmod(path, 0000))
					t.Cleanup(func() { _ = os.Chmod(path, 0600) })
				}
			}
			for _, archived := range []bool{false, true} {
				require.NoError(t, svc.db.Model(peer).Update("is_archived", archived).Error)
				for _, selector := range []struct{ id, name string }{{target.ID, ""}, {"", "target"}} {
					ref, found, used, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), selector.id, selector.name, "")
					require.NoError(t, err, "an unreadable peer is its own problem")
					require.True(t, found)
					require.Equal(t, target.ID, ref.ID)
					require.Nil(t, used, "nothing was overridden")
				}
			}
			// A digest from an older hub is accepted and ignored.
			_, found, _, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", strings.Repeat("a", 64))
			require.NoError(t, err)
			require.True(t, found)
			_, found, _, err = svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), peer.ID, "", "")
			require.Error(t, err)
			require.False(t, found, "the unreadable stack itself stays non-overridable")
		})
	}
}

func TestS4PeerFleetAndLaterDuplicate(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	target := runtimeIdentityProject(t, svc, root, "example-target", "target", identityServices)
	var peers []*Project
	for i := 0; i < 17; i++ {
		peers = append(peers, runtimeIdentityProject(t, svc, root, fmt.Sprintf("example-peer-%02d", i), fmt.Sprintf("peer-%02d", i), "not: [valid"))
	}
	_, found, used, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", "")
	require.NoError(t, err, "any number of unreadable peers is irrelevant")
	require.True(t, found)
	require.Nil(t, used)
	require.NoError(t, os.WriteFile(filepath.Join(peers[1].Path, "compose.yaml"), []byte("name: target\n"+identityServices), 0600))
	_, found, _, err = svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", "")
	require.NoError(t, err)
	require.True(t, found)
	_, _, _, err = svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), "", "target", "")
	require.Equal(t, "compose.duplicate", s4Problem(t, err).Reason)
}

func TestS4DegradedSelectionParityAndMarkerRepair(t *testing.T) {
	for _, mode := range []string{"override", "ordered", "custom", "optional", "valid-env"} {
		t.Run(mode, func(t *testing.T) {
			svc, root := runtimeIdentityFixture(t)
			target := runtimeIdentityProject(t, svc, root, "example-target", "target", identityServices)
			peer := runtimeIdentityProject(t, svc, root, "example-peer", "peer", "name: peer_stack\n"+peerService)
			write := func(name, body string) {
				require.NoError(t, os.WriteFile(filepath.Join(peer.Path, name), []byte(body), 0600))
			}
			switch mode {
			case "override":
				write("compose.override.yaml", "name: target\n")
			case "ordered":
				write("one.yaml", "name: wrong\n"+peerService)
				write("two.yaml", "name: peer_stack\n")
				write("compose.override.yaml", "name: target\n")
				write(".env", "COMPOSE_FILE=one.yaml:two.yaml\n")
			case "custom":
				require.NoError(t, os.Rename(filepath.Join(peer.Path, "compose.yaml"), filepath.Join(peer.Path, "custom.yaml")))
				write("compose.override.yaml", "name: target\n")
			case "optional":
				write("compose.yaml", "name: peer_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n    env_file:\n      - path: service.env\n        required: false\n")
			case "valid-env":
				write("compose.yaml", "name: peer_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n    env_file: [service.env, valid.env]\n")
				write("valid.env", "VALUE=ready\n")
			}
			_, found, _, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", "")
			require.NoError(t, err)
			require.True(t, found, "degraded selection preserves Compose precedence")
			_, _, _, err = svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), "", "target", "")
			if mode == "override" {
				require.Equal(t, "compose.duplicate", s4Problem(t, err).Reason, "the override proves a duplicate name")
			} else {
				require.NoError(t, err)
			}
			if mode == "optional" {
				return
			}
			server := newProjectRuntimeDockerServer(t, nil)
			svc.dockerService = &docker.DockerClientService{Client: newTestDockerClient(t, server)}
			svc.initChildren(nil, nil)
			rows, _, err := svc.ListProjects(t.Context(), pagination.QueryParams{Limit: -1})
			require.NoError(t, err)
			for _, row := range rows {
				if row.ID == peer.ID {
					require.Equal(t, "definition-invalid", row.DefinitionHint)
					require.Equal(t, "compose.env-file-missing", row.DefinitionReason)
					require.Equal(t, "service.env", row.DefinitionFile)
				}
			}
			write("service.env", "VALUE=repaired\n")
			rows, _, err = svc.ListProjects(t.Context(), pagination.QueryParams{Limit: -1})
			require.NoError(t, err)
			for _, row := range rows {
				if row.ID == peer.ID {
					require.Empty(t, row.DefinitionHint)
					require.Empty(t, row.DefinitionReason)
					require.Equal(t, dto.ServiceNotCreated, row.RuntimeServices[0].Status)
				}
			}
		})
	}
}
