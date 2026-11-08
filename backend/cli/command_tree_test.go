// Source: docklet-next/issues/358
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original complete retained hub command tree and alias proof.

package cli

import (
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestM20HubCommandTreeAndAliases(t *testing.T) {
	names := []string{}
	for _, cmd := range rootCmd.Commands() {
		names = append(names, cmd.Name())
	}
	require.ElementsMatch(t, []string{"health", "restore-hub", "version", "generate", "upgrade", "openapi", "config-schema"}, names)
	generate, _, err := rootCmd.Find([]string{"generate"})
	require.NoError(t, err)
	require.Equal(t, []string{"gen", "g"}, generate.Aliases)
	children := []string{}
	for _, cmd := range generate.Commands() {
		children = append(children, cmd.Name())
	}
	require.ElementsMatch(t, []string{"secret", "tls", "mtls", "apikey"}, children)
	for path, name := range map[[2]string]string{{"gen", "api-key"}: "apikey", {"g", "secret"}: "secret", {"generate", "tls"}: "tls", {"generate", "mtls"}: "mtls"} {
		var found *cobra.Command
		found, _, err = rootCmd.Find(path[:])
		require.NoError(t, err)
		require.Equal(t, name, found.Name(), "%v", path)
		require.Equal(t, generate, found.Parent())
	}
}
