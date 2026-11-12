// Source: docklet-next/issues/406
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original typed refusal proof for nested COMPOSE_FILE base files.

package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/updater"
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
