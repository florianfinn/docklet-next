// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"os"

	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	"github.com/spf13/cobra"
)

// Command bypasses application bootstrap and never opens a database or chowns
// files. The target image ENTRYPOINT dispatches this same command in both roles.
func Command() *cobra.Command {
	var id string
	cmd := &cobra.Command{Use: "self-update --op <opId>", Short: "Apply a reserved Compose self-update", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		endpoint := os.Getenv("DOCKER_HOST")
		if endpoint == "" {
			endpoint = "unix:///var/run/docker.sock"
		}
		// This is a standalone process: no inherited shell/config variables may
		// override the project's recorded interpolation file or Compose behaviour.
		os.Clearenv()
		if err := os.Setenv("DOCKER_HOST", endpoint); err != nil {
			return err
		}
		helper, err := core.EmbeddedHelper(endpoint)
		if err != nil {
			return err
		}
		return helper.Run(cmd.Context(), core.HelperData, id)
	}}
	cmd.Flags().StringVar(&id, "op", "", "Reserved self-update operation ID")
	_ = cmd.MarkFlagRequired("op")
	return cmd
}
