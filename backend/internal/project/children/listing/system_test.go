// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original fake Docker system-project tests; hub role.

package listing

import (
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSystemProjectNamesFakeDocker(t *testing.T) {
	require.Equal(t, []string{"system"}, SystemProjectNames([]container.Summary{
		{Labels: map[string]string{"com.docker.compose.project": "system", "io.docklet.system": "agent"}},
		{Labels: map[string]string{"com.docker.compose.project": "docklet-agent"}},
	}))
	require.Equal(t, []string{"docklet"}, SystemProjectNames([]container.Summary{
		{Labels: map[string]string{"com.docker.compose.project": "docklet", "io.docklet.system": "hub"}},
		{Labels: map[string]string{"com.docker.compose.project": "app", "io.docklet.system": "workload"}},
	}))
}
