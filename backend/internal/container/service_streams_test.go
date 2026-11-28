// Source: docklet-next/issues/693
// Revision: 1829fa7776d511dab9604d9b5e3a81cfc7c55640
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original log window test for the container log stream.

package container

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContainerServiceStreamLogsPassesWindowToDockerInternal(t *testing.T) {
	var gotQuery url.Values
	svc := newLogsTestServiceInternal(t, true, []byte("2026-01-02T03:04:05Z line\n"), &gotQuery)
	lines := make(chan string, 4)
	require.NoError(t, svc.StreamLogs(t.Context(), "container-1", lines, false, "all", "2026-01-02T03:00:00Z", "2026-01-02T03:05:00Z", true))
	require.Equal(t, "1767322800", strings.SplitN(gotQuery.Get("since"), ".", 2)[0])
	require.Equal(t, "1767323100", strings.SplitN(gotQuery.Get("until"), ".", 2)[0], "the window end reaches the Docker logs query")
}
