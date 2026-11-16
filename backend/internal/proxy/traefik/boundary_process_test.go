// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original process-boundary assertion for empty-map replacements.

package traefik

import (
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmptyMapReplacementCompletesInTheWorkerProcess(t *testing.T) {
	if os.Getenv("DOCKLET_TRAEFIK_EMPTY_MAP_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestEmptyMapReplacementCompletesInTheWorkerProcess$")
		cmd.Env = append(os.Environ(), "DOCKLET_TRAEFIK_EMPTY_MAP_CHILD=1")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "the worker must finish a valid empty TLS-map update; output bytes=%d", len(output))
		return
	}
	// A runaway recursive replacement must fail this child, not abort its parent
	// test process before it can assert successful completion of the worker.
	debug.SetMaxStack(1 << 20)
	source := strings.Replace(tiny, "      service: app", "      tls: {}\n      service: app", 1)
	d, err := parseDocument(source, "")
	require.NoError(t, err)
	var edits []edit
	require.NoError(t, d.patch(at(d.root, "http", "routers", "app"), map[string]any{"tls": map[string]any{"certResolver": "primary"}}, &edits))
	out, err := d.apply(edits)
	require.NoError(t, err)
	require.Contains(t, out, "tls: {certResolver: primary}")
}
