// Source: docklet-next/docs/design/network-management.md
// Revision: 09d1e632cb3a14bd48b87232acaf991b9d12145e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original host-side network contract implementation for issue #203.

package edge_test

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestNetworkOperationsHaveEdgeCommands(t *testing.T) {
	expected := map[string]string{
		"list-networks": "network.list", "network-counts": "network.counts",
		"create-network": "network.create", "get-network-topology": "network.topology",
		"get-network": "network.inspect", "delete-network": "network.delete",
		"prune-networks": "network.prune", "connect-network-container": "network.connect",
		"disconnect-network-container": "network.disconnect",
	}
	count := 0
	for path, item := range api.SetupAPIForSpec().OpenAPI().Paths {
		if !strings.HasPrefix(path, "/environments/{id}/networks") {
			continue
		}
		for method, op := range map[string]*huma.Operation{"GET": item.Get, "POST": item.Post, "DELETE": item.Delete} {
			if op == nil {
				continue
			}
			count++
			requestPath := "/api" + strings.ReplaceAll(strings.ReplaceAll(path, "{id}", "0"), "{networkId}", "example")
			command, ok := edge.ResolveEdgeCommandName(method, requestPath, false).Get()
			require.True(t, ok, "%s %s (%s)", method, path, op.OperationID)
			require.Contains(t, expected, op.OperationID)
			require.Equal(t, expected[op.OperationID], command)
			require.True(t, edge.ValidateEdgeCommand(command, method, requestPath, false))
			require.Contains(t, edge.AdvertisedEdgeCommands(), command)
		}
	}
	require.Equal(t, 9, count)
}
