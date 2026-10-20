// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Activated runtime metadata shares the bounded Edge command vocabulary.

package edge

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRuntimeEdgeActivatedCommandVocabulary(t *testing.T) {
	for _, tc := range []struct{ method, path, command string }{
		{"POST", "/api/environments/0/runtime-registration", "runtime.register"},
		{"POST", "/api/environments/0/runtime-target", "runtime.resolve"},
		{"POST", "/api/environments/0/runtime-runs", "runtime.execute"},
		{"GET", "/api/environments/0/runtime-runs/example-run", "runtime.query"},
		{"POST", "/api/environments/0/runtime-runs/example-run/cancel", "runtime.cancel"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			name, ok := ResolveEdgeCommandName(tc.method, tc.path, false).Get()
			require.True(t, ok)
			require.Equal(t, tc.command, name)
			require.True(t, ValidateEdgeCommand(tc.command, tc.method, tc.path, false))
			name, ok = RuntimeCommandName(tc.method, tc.path)
			require.True(t, ok)
			require.Equal(t, tc.command, name)
			require.True(t, runtimeRequestPathInternal(tc.path))
			require.True(t, runtimeCommandValidInternal(tc.command, tc.method, tc.path))
			require.False(t, runtimeCommandValidInternal("container.start", tc.method, tc.path))
			_, ok = ResolveEdgeCommandName("DELETE", tc.path, false).Get()
			require.False(t, ok)
			_, ok = RuntimeCommandName("DELETE", tc.path)
			require.False(t, ok)
		})
	}
}
