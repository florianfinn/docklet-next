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

const volumeSentence = "Volume data was not restored; the app may already have migrated it."

func TestScheduledRecoveryNoticeText(t *testing.T) {
	for _, kind := range []string{"stack", "container"} {
		for _, tc := range []struct{ reason, recovery, head, detail string }{
			{"auto-restored", "restored", "Scheduled update failed", "The previous version is running again."},
			{"auto-restore-failed", "restore-failed", "Scheduled update failed", "The automatic restore failed too — action needed."},
			{"auto-restore-unavailable", "unavailable", "Scheduled update failed", "An automatic restore was not possible (--rm) — action needed."},
			{"update-interrupted", "interrupted", "Scheduled update interrupted", "Check the target."},
			// not-needed proves no workload effect: no volume warning.
			{"runtime.failed", "not-needed", "Scheduled update failed", "Reason: the deployment failed"},
			{"update-interrupted", "not-needed", "Scheduled update interrupted", "Check the target."},
		} {
			t.Run(kind+"/"+tc.reason+"/"+tc.recovery, func(t *testing.T) {
				payload := database.JSON{kind: "app", "operation": "update-project", "reason": tc.reason, "recovery": tc.recovery, "recoveryReason": "container.auto-remove", "scheduled": true}
				text := outboxText("deploy_failed", "Example host", payload, "")
				want := tc.head + ": " + kind + " app on Example host\n" + tc.detail
				if tc.recovery != "not-needed" {
					want += "\n" + volumeSentence
				}
				require.Equal(t, want, text)
				withLink := outboxText("deploy_failed", "Example host", payload, "https://docklet.example.com")
				require.True(t, strings.HasPrefix(withLink, text+"\nOpen Docklet:"))
			})
		}
	}
}

func TestManualRecoveryNoticeTextUnchanged(t *testing.T) {
	require.Equal(t, "Update failed: stack app on Example host\nReason: the deployment failed", outboxText("deploy_failed", "Example host", database.JSON{"stack": "app", "operation": "update-project", "reason": "runtime.failed"}, ""))
	require.Equal(t, "Update failed: container app on Example host\nReason: the deployment failed", outboxText("deploy_failed", "Example host", database.JSON{"container": "app", "operation": "update-container", "reason": "runtime.failed"}, ""))
}
