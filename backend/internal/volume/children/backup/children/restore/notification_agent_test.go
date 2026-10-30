// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package restore

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/notification/backupnotify"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLiveAgentRestoreFailureDispatchesAfterTerminalSave(t *testing.T) {
	for _, kind := range []string{"full", "files", "upload", "new-volume"} {
		t.Run(kind, func(t *testing.T) {
			l, row := lifecycleFixture(t)
			require.NoError(t, l.DB.AutoMigrate(&notificationoutbox.Row{}))
			calls := 0
			l.Notify = backupnotify.Notifier{DB: l.DB, Agent: true, Dispatch: func(ctx context.Context, p dto.DispatchRequest) error {
				calls++
				var saved Run
				require.NoError(t, l.DB.First(&saved, "id=?", row.ID).Error)
				require.Equal(t, "failed", saved.Status)
				require.Equal(t, "restore", p.BackupFailed.Operation)
				return errors.New("controlled unreachable manager")
			}}.Terminal
			row.Status = "failed"
			row.ErrorCode = "volume.restore.failed"
			row.Kind = kind
			if kind == "new-volume" {
				row.Kind = "full"
				row.Mode = "new"
				row.NewVolumeName = "restored-data"
			}
			l.persistTerminal(t.Context(), t.Context(), row)
			require.Equal(t, 1, calls)
			var n int64
			require.NoError(t, l.DB.Model(&notificationoutbox.Row{}).Count(&n).Error)
			require.Zero(t, n)
		})
	}
}
