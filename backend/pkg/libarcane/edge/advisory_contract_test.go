// Source: docklet-next/issues/437
// Revision: 2c49b8db789d39b644e27a85a09e277707b4fe12
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original advisory allowlist and advertised command assertion. M21 #437: advisory detail.

package edge_test

import (
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/vulnerability"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAdvisoryCommandAllowlisted(t *testing.T) {
	path := "/api/environments/0/vulnerabilities/advisories/" + vulnerability.AdvisoryKey("vendor/advisory")
	command, ok := edge.ResolveEdgeCommandName("GET", path, false).Get()
	require.True(t, ok)
	require.Equal(t, "vulnerability.advisory", command)
	require.True(t, edge.ValidateEdgeCommand(command, "GET", path, false))
	require.Contains(t, edge.AdvertisedEdgeCommands(), command)
	require.False(t, edge.ValidateEdgeCommand(command, "POST", path, false))
	require.False(t, edge.ValidateEdgeCommand(command, "GET", path, true))
}
