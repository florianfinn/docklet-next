// Source: docklet-next/docs/design/storage-workspace.md
// Revision: ce3c4ea4af5b0bc1cdb2bb287d30281bbe20f0a8
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original R1 correction for issue #179.
package volume

import "fmt"

// helperCommandExitError means Docker completed the command; transport failures
// stay separate so callers do not infer image incompatibility from them.
type helperCommandExitError struct {
	code   int
	output string
}

func (e *helperCommandExitError) Error() string {
	if e.output != "" {
		return fmt.Sprintf("command exited with code %d: %s", e.code, e.output)
	}
	return fmt.Sprintf("command exited with code %d", e.code)
}
