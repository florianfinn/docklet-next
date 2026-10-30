// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original shared M14 schema registration guard.

package api

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestContainerHostContractSchemas(t *testing.T) {
	api := SetupAPIForSpec()
	for _, name := range []string{"ContainerOriginAssignment", "ContainerRecreateRequest", "ContainerProblem", "ContainerShellQuery", "ContainerShellResize", "ContainerShellExit", "EnvironmentAgentRestartResponse", "EnvironmentAgentRestartRun", "EnvironmentHostProblem"} {
		require.Contains(t, api.OpenAPI().Components.Schemas.Map(), name)
	}
}
