// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original fake Docker agent-container tests.

package container

import (
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAgentAlwaysHiddenFakeDocker(t *testing.T) {
	containers := []container.Summary{{ID: "agent", Labels: map[string]string{"io.docklet.system": "agent"}}, {ID: "user"}}
	for _, flags := range [][2]bool{{false, false}, {true, true}, {true, false}, {false, true}} {
		got := FilterExcludedContainers(containers, flags[0], flags[1])
		require.Len(t, got, 1)
		require.Equal(t, "user", got[0].ID)
	}
}
