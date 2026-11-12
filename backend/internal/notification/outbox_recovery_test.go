// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original outbound English recovery notice contract proofs.

package notification

import (
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestScheduledRecoveryNoticeText(t *testing.T) {
	for _, kind := range []string{"stack", "container"} {
		for _, tc := range []struct{ reason, phrase string }{
			{"auto-restored", "The previous version is running again."},
			{"auto-restore-failed", "the automatic restore failed too — action needed."},
			{"auto-restore-unavailable", "an automatic restore was not possible (--rm) — action needed."},
			{"update-interrupted", "was interrupted — check the target."},
			{"runtime.failed", "Reason: the deployment failed"},
		} {
			t.Run(kind+"/"+tc.reason, func(t *testing.T) {
				payload := database.JSON{kind: "app", "operation": "update-project", "reason": tc.reason, "recoveryReason": "container.auto-remove", "scheduled": true}
				text := outboxText("deploy_failed", "Example host", payload, "")
				if tc.reason == "runtime.failed" {
					require.True(t, strings.HasPrefix(text, "Update failed: "+kind+" app on Example host\nReason: the deployment failed\n"))
				} else {
					require.True(t, strings.HasPrefix(text, "Scheduled update of "+kind+" app on Example host"))
				}
				require.Contains(t, text, tc.phrase)
				require.True(t, strings.HasSuffix(text, "Volume data was not restored; the app may already have migrated it."))
				withLink := outboxText("deploy_failed", "Example host", payload, "https://docklet.example.com")
				require.Contains(t, withLink, text+"\nOpen Docklet:")
			})
		}
	}
}

func TestManualRecoveryNoticeTextUnchanged(t *testing.T) {
	require.Equal(t, "Update failed: stack app on Example host\nReason: the deployment failed", outboxText("deploy_failed", "Example host", database.JSON{"stack": "app", "operation": "update-project", "reason": "runtime.failed"}, ""))
	require.Equal(t, "Update failed: container app on Example host\nReason: the deployment failed", outboxText("deploy_failed", "Example host", database.JSON{"container": "app", "operation": "update-container", "reason": "runtime.failed"}, ""))
}
