// Source: getarcaneapp/arcane/cli/pkg/generate/generate.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Moved into the hub command implementation; certificate builders use the non-command certgen package; M20 removal of inbound webhooks and remote CLI, retaining hub commands and typed refusals.

package generate

import (
	"github.com/spf13/cobra"
)

var GenerateCmd = &cobra.Command{
	Use:     "generate",
	Aliases: []string{"gen", "g"},
	Short:   "Generate secrets and bootstrap credentials for Arcane",
}
