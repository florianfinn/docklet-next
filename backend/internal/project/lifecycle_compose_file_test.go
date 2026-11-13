// Source: docklet-next/issues/406
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original typed refusal proof for nested COMPOSE_FILE base files.

package project

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// A base file in a subdirectory is refused as an unsupported layout before any read
// of the model or any image, file or deploy effect; other COMPOSE_FILE errors keep
// the generic snapshot code.
func TestLifecycleRefusesNestedComposeFileBaseAsUnsupported(t *testing.T) {
	for _, tc := range []struct {
		name, selection, code string
	}{
		{"nested base", "deploy/compose.yaml", "update.image-binding-unsupported"},
		{"nested later entry", "compose.yaml:deploy/extra.yaml", "update.image-binding-unsupported"},
		{"missing entry", "compose.yaml:deploy/missing.yaml", "update.snapshot-unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLifecycleFixture(t)
			dir := filepath.Join(f.project.Path, "deploy")
			require.NoError(t, os.Mkdir(dir, 0700))
			nested := "services:\n  app:\n    image: registry.example.com/app:1.0.0\n"
			require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(nested), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.yaml"), []byte("services:\n  app:\n    environment:\n      EXTRA: \"1\"\n"), 0600))
			env := "STACK_INPUT=original\nCOMPOSE_FILE=" + tc.selection + "\n"
			require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, ".env"), []byte(env), 0600))

			_, _, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
			lifecycleCode(t, err, tc.code)
			intent, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: strings.Repeat("0", 64)})
			require.Nil(t, intent)
			lifecycleCode(t, err, tc.code)

			require.Zero(t, f.pulls)
			require.Zero(t, f.tagWrites)
			require.Zero(t, f.deploys)
			for path, want := range map[string]string{filepath.Join(dir, "compose.yaml"): nested, filepath.Join(f.project.Path, ".env"): env, filepath.Join(f.project.Path, "compose.yaml"): lifecycleOriginal} {
				got, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, want, string(got))
			}
		})
	}
}

// Update checks refuse a nested base before reading details; a peer's nested base
// keeps blocking identity lookups but never reads as the target's own refusal.
func TestNestedComposeFileRefusalForChecksAndIdentity(t *testing.T) {
	f := newLifecycleFixture(t)
	require.NoError(t, f.service.UpdateLayoutRefusal(t.Context(), f.project.ID))
	peerPath := filepath.Join(f.root, "example-peer")
	require.NoError(t, os.MkdirAll(filepath.Join(peerPath, "deploy"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(peerPath, "deploy", "compose.yaml"), []byte("services:\n  app:\n    image: registry.example.com/app:1.0.0\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(peerPath, ".env"), []byte("COMPOSE_FILE=deploy/compose.yaml\n"), 0600))
	peer := &Project{BaseModel: database.BaseModel{ID: uuid.NewString()}, Name: "example-peer", Path: peerPath, DirName: new("example-peer")}
	require.NoError(t, f.db.Create(peer).Error)

	lifecycleCode(t, f.service.UpdateLayoutRefusal(t.Context(), peer.ID), "update.image-binding-unsupported")
	_, _, err := f.service.ResolveRuntimeProjectIdentity(t.Context(), peer.ID, "")
	require.True(t, errors.Is(err, projects.ErrComposeFileNestedBase), "%v", err)
	_, _, err = f.service.ResolveRuntimeProjectIdentity(t.Context(), f.project.ID, "")
	require.Error(t, err)
	require.False(t, errors.Is(err, projects.ErrComposeFileNestedBase), "a peer's layout must not read as the target's refusal")
}
