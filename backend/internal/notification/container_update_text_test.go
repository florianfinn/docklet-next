// Source: docklet-next/issues/443
// Revision: fddfe5d38eacf1a387b773ce10d41384f8c01459
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original container image update failure notice text tests.

package notification

import (
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/stretchr/testify/require"
)

func TestContainerUpdateFailureNoticeNamesTheContainer(t *testing.T) {
	for _, tc := range []struct{ operation, title string }{
		{"update-container", "Update failed: container app on Example"},
		{"restore-container-image", "Restore failed: container app on Example"},
	} {
		text := outboxText("deploy_failed", "Example", database.JSON{"host": "Example", "container": "app", "operation": tc.operation, "reason": "runtime.failed"}, "https://docklet.example.com")
		lines := strings.Split(text, "\n")
		require.Equal(t, tc.title, lines[0])
		require.Equal(t, "Open Docklet: https://docklet.example.com/containers", lines[len(lines)-1])
		require.NotContains(t, text, "stack")
	}
	stack := outboxText("deploy_failed", "Example", database.JSON{"host": "Example", "stack": "web", "operation": "update-project", "reason": "runtime.failed"}, "https://docklet.example.com")
	require.True(t, strings.HasPrefix(stack, "Update failed: stack web on Example\n"))
	require.True(t, strings.HasSuffix(stack, "/stacks"))
}
