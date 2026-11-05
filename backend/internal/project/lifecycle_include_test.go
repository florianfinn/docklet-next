// Source: docklet-next/issues/344
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original include-stack capture refusal proof.

package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The loader keeps only the root file of an include stack. An included file without
// services passes every service check, so capture must refuse the include itself instead
// of recording a restore point or baseline without that file.
func TestLifecycleCaptureRefusesIncludeStacks(t *testing.T) {
	f := newLifecycleFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "included.yaml"), []byte("networks:\n  shared:\n    name: example-shared\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte("include:\n  - included.yaml\n"+lifecycleOriginal), 0600))
	_, err := f.service.lifecycleInputs(t.Context(), f.project)
	lifecycleCode(t, err, "update.image-binding-unsupported")
}
