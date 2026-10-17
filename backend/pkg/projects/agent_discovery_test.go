// Source: docklet-next/docs/design/host-onboarding.md
// Revision: 81439fab6a8ec5232161dc933fbd41bec41885fa
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original agent installation discovery tests.

package projects

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverySkipsAgentInstallDirectory(t *testing.T) {
	for _, labels := range []string{`{io.docklet.system: agent}`, `["io.docklet.system=agent"]`} {
		t.Run(labels, func(t *testing.T) {
			root := t.TempDir()
			agent := filepath.Join(root, "docklet-agent")
			require.NoError(t, os.MkdirAll(agent, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(agent, "compose.yaml"), []byte("services:\n  agent:\n    image: docklet-agent:0.0.0\n    labels: "+labels+"\n"), 0o644))
			writeComposeFileInternal(t, filepath.Join(agent, "data", "nested"))
			writeComposeFileInternal(t, filepath.Join(root, "normal"))
			found, err := DiscoverProjectDirectories(t.Context(), root, false, 3)
			require.NoError(t, err)
			require.Equal(t, []string{"normal"}, discoveredNamesInternal(found))
		})
	}
}

func TestDiscoveryKeepsMixedOrUnlabelledStack(t *testing.T) {
	for _, content := range []string{
		"services: {app: {image: example:0.0.0}}",
		"services: {agent: {labels: {io.docklet.system: agent}}, app: {image: example:0.0.0}}",
		"services: {app: {labels: {io.docklet.system: other}}}",
	} {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(content), 0o644))
		found, err := DiscoverProjectDirectories(t.Context(), root, false, 3)
		require.NoError(t, err)
		require.Len(t, found, 1)
	}
}
