// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package backup

import (
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestActualArchiveFinalizerExpiresUnsentNotices(t *testing.T) {
	source := restoreArchiveNew(t)
	source.seed(t)
	now := time.Now().UTC()
	for _, status := range []string{"pending", "sending", "sent"} {
		r := notificationoutbox.Row{ID: status, Event: "backup_failed", Payload: database.JSON{}, EnvironmentID: "0", DedupKey: status, CreatedAt: now, Status: status}
		if status == "sending" {
			r.LeaseUntil = &now
		}
		require.NoError(t, source.db.Create(&r).Error)
	}
	require.NoError(t, source.db.Create(&notificationoutbox.Availability{EnvironmentID: "remote", OfflineSince: now}).Error)
	record := restoreArchiveMetadata(t, source)
	target := restoreArchiveInstall(t, source, record)
	require.NoError(t, finalizeRestoredDatabase(t.Context(), target.db, target.cfg))
	var rows []notificationoutbox.Row
	require.NoError(t, target.db.Order("id").Find(&rows).Error)
	require.Len(t, rows, 3)
	require.Equal(t, []string{"expired", "expired", "sent"}, []string{rows[0].Status, rows[1].Status, rows[2].Status})
	var n int64
	require.NoError(t, target.db.Model(&notificationoutbox.Availability{}).Count(&n).Error)
	require.Zero(t, n)
}
