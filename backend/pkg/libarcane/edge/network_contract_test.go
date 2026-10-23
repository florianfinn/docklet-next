// Source: docklet-next/docs/design/network-management.md
// Revision: 09d1e632cb3a14bd48b87232acaf991b9d12145e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original host-side network contract implementation for issue #203.

package edge_test

import (
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/stretchr/testify/require"
	"os"
	"slices"
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

func TestNetworkV6FixtureMatchesRegisteredProducer(t *testing.T) {
	body, err := os.ReadFile("../../../../types/contract/testdata/agent-hub-v6.json")
	require.NoError(t, err)
	var fixture struct {
		Network struct {
			Routes     []struct{ Method, Path, OperationID string }
			Fields     map[string][]string
			ErrorCodes []string
		}
	}
	require.NoError(t, json.Unmarshal(body, &fixture))
	producer := api.SetupAPIForSpec().OpenAPI()
	require.Len(t, fixture.Network.Routes, 9)
	for _, route := range fixture.Network.Routes {
		item := producer.Paths[route.Path]
		require.NotNil(t, item)
		op := map[string]*huma.Operation{"GET": item.Get, "POST": item.Post, "DELETE": item.Delete}[route.Method]
		require.NotNil(t, op)
		require.Equal(t, route.OperationID, op.OperationID)
	}
	for name, want := range fixture.Network.Fields {
		schema := producer.Components.Schemas.Map()[name]
		require.NotNil(t, schema, name)
		actual := []string{}
		for field := range schema.Properties {
			if field != "$schema" {
				actual = append(actual, field)
			}
		}
		slices.Sort(actual)
		require.Equal(t, want, actual, name)
	}
	actual := []string{}
	for _, code := range producer.Components.Schemas.Map()["NetworkError"].Properties["code"].Enum {
		actual = append(actual, code.(string))
	}
	slices.Sort(actual)
	require.Equal(t, fixture.Network.ErrorCodes, actual)
}
