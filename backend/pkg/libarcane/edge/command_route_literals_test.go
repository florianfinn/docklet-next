// Source: docklet-next/issues/264
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original structural proof that Edge command route literals use unreserved characters only.

package edge

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The Edge command table must satisfy the same literal rule as the hub and agent routers (#264).
func TestCommandRouteLiteralSegmentsAreUnreserved(t *testing.T) {
	unreserved := regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)
	require.NotEmpty(t, commandRoutes)
	for _, route := range commandRoutes {
		for _, segment := range strings.Split(strings.TrimPrefix(route.PathPattern, "/"), "/") {
			if strings.HasPrefix(segment, "{") {
				continue
			}
			require.True(t, unreserved.MatchString(segment) && segment != "." && segment != "..",
				"%s %s has literal %q", route.Method, route.PathPattern, segment)
		}
	}
}

// Command resolution uses the path as the hub checked it: trailing whitespace
// is not trimmed into a different literal (#264, the hub refuses such paths).
func TestCommandResolutionDoesNotTrimThePath(t *testing.T) {
	require.Equal(t, "container.counts", ResolveEdgeCommandName(http.MethodGet, "/api/environments/1/containers/counts", false).OrEmpty())
	for _, path := range []string{"/api/environments/1/containers/counts ", "/api/environments/1/containers/counts ", " /api/environments/1/containers/counts"} {
		require.NotEqual(t, "container.counts", ResolveEdgeCommandName(http.MethodGet, path, false).OrEmpty(), "%q", path)
		require.False(t, ValidateEdgeCommand("container.counts", http.MethodGet, path, false), "%q", path)
	}
}
