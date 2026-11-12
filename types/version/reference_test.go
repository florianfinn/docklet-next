// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original release reference predicate tests.

package version

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReleaseReference(t *testing.T) {
	digest := "@sha256:" + strings.Repeat("a", 64)
	for _, ref := range []string{
		"ghcr.io/florianfinn/docklet-next-agent:1.0.0" + digest,
		"ghcr.io/florianfinn/docklet-next-agent:1.0.0-rc.3" + digest,
		"registry.example.com:5000/docklet/agent:0.90.1" + digest,
		"localhost:5000/docklet-next-agent:1.0.0" + digest,
	} {
		require.True(t, ReleaseReference(ref), ref)
	}
	for _, ref := range []string{
		"",
		"ghcr.io/florianfinn/docklet-next-agent:1.0.0",
		"ghcr.io/florianfinn/docklet-next-agent:latest" + digest,
		"ghcr.io/florianfinn/docklet-next-agent:v1.0.0" + digest,
		"ghcr.io/florianfinn/docklet-next-agent:1.0" + digest,
		"ghcr.io/florianfinn/docklet-next-agent:01.0.0" + digest,
		"ghcr.io/florianfinn/docklet-next-agent:1.0.0-" + digest,
		"ghcr.io/florianfinn/docklet-next-agent" + digest,
		"docklet-next-agent:1.0.0" + digest,
		"ghcr.io/Florianfinn/agent:1.0.0" + digest,
		"ghcr.io/florianfinn/docklet-next-agent:1.0.0@sha256:" + strings.Repeat("A", 64),
		"ghcr.io/florianfinn/docklet-next-agent:1.0.0@sha256:abc",
		"ghcr.io/florianfinn/docklet-next-agent:1.0.0@sha512:" + strings.Repeat("a", 64),
	} {
		require.False(t, ReleaseReference(ref), ref)
	}
}
