// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

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
	// Unknown rows may precede a duplicate; the full scan must find the hard block.
	unknown := runtimeIdentityProject(t, svc, root, "example-unknown", "unknown", "name: ${PEER_NAME:?required}\n"+identityServices)
	_, _, _, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", "")
	digest := s4Problem(t, err).Uncertainty.Digest
	require.NoError(t, os.WriteFile(filepath.Join(peer.Path, "compose.yaml"), []byte("name: target_stack\n"+peerService), 0600))
	for _, selector := range []struct{ id, name string }{{target.ID, ""}, {"", "target_stack"}} {
		_, found, _, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), selector.id, selector.name, digest)
		b := s4Problem(t, err)
		require.False(t, found)
		require.False(t, b.Confirmable, "duplicate must win over an acknowledged unknown peer")
		require.Equal(t, "compose.duplicate", b.Reason)
	}
	require.NoError(t, os.WriteFile(filepath.Join(peer.Path, "service.env"), []byte("VALUE=ready\n"), 0600))
	_, _, _, err = svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", digest)
	require.False(t, s4Problem(t, err).Confirmable, "readable duplicates remain hard")
	require.NoError(t, svc.db.Delete(unknown).Error)
	// A missing service environment remains a non-overridable target failure.
	require.NoError(t, os.Remove(filepath.Join(peer.Path, "service.env")))
	_, found, _, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), peer.ID, "", digest)
	require.Error(t, err)
	require.False(t, found)
}

func TestS4PeerUncertaintyClassesAndFreshAcknowledgement(t *testing.T) {
	for _, tc := range []struct{ name, compose, env, extra, reason string }{
		{"interpolation", "name: ${PEER_NAME:?required}\n" + identityServices, "", "", "compose.identity-unreadable"},
		{"yaml", "not: [valid", "", "", "compose.identity-unreadable"},
		{"primary-env", identityServices, "VALUE=\"unterminated\n", "", "compose.identity-unreadable"},
		{"selection", identityServices, "COMPOSE_FILE=nested/compose.yaml\n", "nested/compose.yaml", "compose.selection-unsupported"},
		{"mixed-env", "name: peer_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n    env_file: [service.env, invalid.env]\n", "", "invalid.env", "compose.env-file-missing"},
		{"label-file", "name: peer_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n    label_file: labels.env\n", "", "", "compose.identity-unreadable"},
		{"include", "include: [missing.yaml]\n" + identityServices, "", "", "compose.identity-unreadable"},
		{"permission", "name: peer_stack\n" + peerService, "", "service.env", "compose.identity-unreadable"},
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
			require.NoError(t, svc.db.Model(peer).Update("is_archived", true).Error)
			_, found, _, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", "")
			require.False(t, found)
			b := s4Problem(t, err)
			require.True(t, b.Confirmable, "unknown peers require explicit confirmation")
			require.Equal(t, tc.reason, b.Uncertainty.Peers[0].Reason)
			ref, found, used, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", b.Uncertainty.Digest)
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, target.ID, ref.ID)
			require.Equal(t, 1, used.Count)
			require.True(t, used.Peers[0].Archived)
			_, _, _, err = svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", strings.Repeat("a", 64))
			require.True(t, s4Problem(t, err).Confirmable, "wrong digest must not acknowledge current evidence")
			_, found, _, err = svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), peer.ID, "", b.Uncertainty.Digest)
			require.Error(t, err)
			require.False(t, found, "target failures cannot be confirmed")
		})
	}
}

func TestS4UncertaintyFleetBoundaryAndIdentityChanges(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	target := runtimeIdentityProject(t, svc, root, "example-target", "target", identityServices)
	var peers []*Project
	for i := 0; i < 17; i++ {
		peers = append(peers, runtimeIdentityProject(t, svc, root, fmt.Sprintf("example-peer-%02d", i), fmt.Sprintf("peer-%02d", i), "not: [valid"))
		if i != 15 && i != 16 {
			continue
		}
		_, _, _, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", "")
		b := s4Problem(t, err)
		require.Equal(t, i+1, b.Uncertainty.Count)
		require.Len(t, b.Uncertainty.Peers, 16)
		_, found, used, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", b.Uncertainty.Digest)
		require.NoError(t, err, "one confirmation must cover the complete fleet")
		require.True(t, found)
		require.Equal(t, i+1, used.Count)
	}
	_, _, _, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", "")
	original := s4Problem(t, err).Uncertainty.Digest
	// Add, repair, recreate, and reason changes each invalidate the original digest.
	extra := runtimeIdentityProject(t, svc, root, "example-zz-later", "later", "not: [valid")
	_, _, _, err = svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", original)
	require.NotEqual(t, original, s4Problem(t, err).Uncertainty.Digest, "new peers need a new deliberate confirmation")
	require.NoError(t, svc.db.Delete(extra).Error)
	require.NoError(t, svc.db.Model(peers[0]).Update("id", "example-recreated").Error)
	_, _, _, err = svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", original)
	require.NotEqual(t, original, s4Problem(t, err).Uncertainty.Digest, "recreated peer cannot inherit acknowledgement")
	require.NoError(t, os.WriteFile(filepath.Join(peers[1].Path, "compose.yaml"), []byte(identityServices), 0600))
	_, _, _, err = svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", original)
	require.Equal(t, 16, s4Problem(t, err).Uncertainty.Count, "repaired peers are read again")
	require.NoError(t, os.WriteFile(filepath.Join(peers[1].Path, "compose.yaml"), []byte("name: target\n"+identityServices), 0600))
	_, _, _, err = svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), target.ID, "", original)
	require.False(t, s4Problem(t, err).Confirmable, "a later proven duplicate can never be acknowledged")
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
			if mode == "override" {
				require.False(t, s4Problem(t, err).Confirmable)
			} else {
				require.NoError(t, err)
				require.True(t, found, "degraded selection preserves Compose precedence")
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
