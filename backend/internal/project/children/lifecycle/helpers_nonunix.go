// Source: getarcaneapp/arcane/backend/internal/project/children/lifecycle/helpers_nonunix.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

//go:build !unix

package lifecycle

import (
	"fmt"
)

func describeLifecyclePathAccess(projectPath, resolvedScriptPath string) string {
	return fmt.Sprintf("Project path=%q, resolved script path=%q. Ensure the Arcane process can traverse every parent "+
		"directory and inspect the script. A script being executable is not sufficient when ownership, ACLs, "+
		"or mount permissions block access.", projectPath,
		resolvedScriptPath,
	)
}
