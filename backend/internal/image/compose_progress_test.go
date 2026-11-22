// Source: docklet-next/issues/567
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original bounded JSON observer and unchanged renderer evidence.

package image

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/docker"
)

func TestComposePullObservationPreservesRawRendering(t *testing.T) {
	raw := `{"id":"abcdef012345","status":"Downloading","progressDetail":{"current":1,"total":2}}` + "\n" + `{"id":"abcdef012345","status":"Pull complete"}` + "\n"
	var patch map[string]any
	reporter := projects.NewDeployReporter(func(string, bool) {}, func(p map[string]any) { patch = p }, time.Now)
	reader := &composePullReader{source: strings.NewReader(raw), reporter: reporter, image: "image"}
	var original, observed strings.Builder
	require.NoError(t, docker.RenderJSONMessageStream(strings.NewReader(raw), &original))
	require.NoError(t, docker.RenderJSONMessageStream(reader, &observed))
	reporter.Flush(true)
	require.Equal(t, original.String(), observed.String(), "observation must preserve the existing non-TTY renderer")
	require.Equal(t, 1, patch["deployPull"].(map[string]any)["layersDone"])
	// Bounded observation never drops source bytes, including malformed oversized lines.
	large := strings.Repeat("x", 65537) + "\n" + raw
	source := &composePullReader{source: strings.NewReader(large), reporter: reporter, image: "second"}
	actual, err := io.ReadAll(source)
	require.NoError(t, err)
	require.Equal(t, large, string(actual))
	require.LessOrEqual(t, cap(source.pending), 65536)
}
