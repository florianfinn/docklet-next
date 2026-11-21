// Source: docklet-next/docs/design/stack-env-and-peers.md
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original peer name evidence tests for issue 547.

package projects

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPeerLiteralName(t *testing.T) {
	const services = "services:\n  app:\n    image: registry.example.com/app:${TAG:?set in an env file}\n"
	for _, tc := range []struct {
		name, compose, env, extra, want string
	}{
		{"literal", "name: edge\n" + services, "", "", "edge"},
		{"normalized", "name: Edge_Stack\n" + services, "", "", "edge_stack"},
		{"interpolated", "name: ${STACK}\n" + services, "", "", ""},
		{"bare-dollar", "name: $STACK\n" + services, "", "", ""},
		{"escaped-dollar", "name: a$$b\n" + services, "", "", ""},
		{"quoted", "name: \"edge\"\n" + services, "", "", ""},
		{"alias", "x: &n edge\nname: *n\n" + services, "", "", ""},
		{"multi-document", "name: edge\n" + services + "---\nname: other\n", "", "", ""},
		{"invalid", "name: [edge", "", "", ""},
		{"absent", services, "", "", ""},
		{"selection", "name: edge\n" + services, "COMPOSE_FILE=other.yaml\n", "", ""},
		{"project-name", "name: edge\n" + services, "export COMPOSE_PROJECT_NAME=other\n", "", ""},
		{"override", "name: edge\n" + services, "", "compose.override.yaml", ""},
		{"two-bases", "name: edge\n" + services, "", "docker-compose.yml", ""},
		{"unrelated-env", "name: edge\n" + services, "TAG=1\n", "", "edge"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(tc.compose), 0600))
			if tc.env != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(tc.env), 0600))
			}
			if tc.extra != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, tc.extra), []byte("name: target\n"), 0600))
			}
			require.Equal(t, tc.want, PeerLiteralName(dir))
		})
	}
	dir := t.TempDir()
	require.Empty(t, PeerLiteralName(dir), "no compose file")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("name: edge\n"+services), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), nil, 0))
	if os.Geteuid() != 0 {
		require.Empty(t, PeerLiteralName(dir), "unreadable .env gives no evidence")
	}
}
