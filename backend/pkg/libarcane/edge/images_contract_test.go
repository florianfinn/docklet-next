// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Image fixture binding to the real Edge command table for issue #229.

package edge_test

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/stretchr/testify/require"
	"os"
	"strings"
	"testing"
)

func TestImageV7FixtureMatchesEdgeCommandProducer(t *testing.T) {
	data, err := os.ReadFile("../../../../types/contract/testdata/agent-hub-v7.json")
	require.NoError(t, err)
	var fixture struct {
		ImageSafety struct {
			EdgeCommands []string
			EdgeRoutes   []struct{ Method, Path, Command string }
		}
	}
	require.NoError(t, json.Unmarshal(data, &fixture))
	covered := map[string]bool{}
	for _, route := range fixture.ImageSafety.EdgeRoutes {
		path := strings.NewReplacer("{id}", "0", "{imageId}", "example", "{name}", "example").Replace(route.Path)
		command, ok := edge.ResolveEdgeCommandName(route.Method, path, false).Get()
		require.True(t, ok, "%s %s", route.Method, path)
		require.Equal(t, route.Command, command)
		require.True(t, edge.ValidateEdgeCommand(command, route.Method, path, false))
		require.Contains(t, edge.AdvertisedEdgeCommands(), route.Command)
		require.Contains(t, fixture.ImageSafety.EdgeCommands, route.Command)
		covered[route.Command] = true
	}
	require.Len(t, covered, len(fixture.ImageSafety.EdgeCommands))
	require.True(t, covered["image.counts"])
	require.True(t, covered["image_update.summary"])
}
