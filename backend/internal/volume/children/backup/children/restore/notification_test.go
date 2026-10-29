// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package restore

import (
	"github.com/florianfinn/docklet-next/backend/internal/notification/backupnotify"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRestoreTerminalNoticeAndSilentHistory(t *testing.T) {
	l, _ := lifecycleFixture(t) // existing fake effects and migrated restore store
	require.NoError(t, l.DB.AutoMigrate(&notificationoutbox.Row{}))
	n := backupnotify.Notifier{DB: l.DB}
	l.Notify = n.Terminal
	for _, kind := range []string{"full", "files", "upload"} {
		row := &Run{ID: kind, VolumeName: "data", Kind: kind, Phase: "done", Status: "failed", ErrorCode: "volume.restore.failed", StartedAt: time.Now().UTC()}
		l.persistTerminal(t.Context(), row)
	}
	var rows []notificationoutbox.Row
	require.NoError(t, l.DB.Find(&rows).Error)
	require.Len(t, rows, 3)
	require.NoError(t, l.Reconcile(t.Context(), false, nil))
	require.NoError(t, l.DB.Find(&rows).Error)
	require.Len(t, rows, 3, "startup repair is silent")
}
