// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M13 closing-verification regressions.

package backup

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestHubBackupDeterminedFailureNotifiesWhenCancelledWhileSaved(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			g, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
			require.NoError(t, err)
			db := &database.DB{DB: g}
			require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}))
			require.NoError(t, db.AutoMigrate(activity.Models()...))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			saved := false
			s := NewService(Dependencies{DB: db, Config: &config.Config{}, Activity: activity.NewActivityService(db, nil), Store: Store{SaveRun: func(context.Context, *dto.SystemBackupRun) error {
				saved = true
				if cancelled {
					cancel()
				}
				return nil
			}}})
			require.Error(t, s.finishRun(ctx, &dto.SystemBackupRun{ID: "run", Trigger: "manual"}))
			require.True(t, saved)
			var n int64
			require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
			// The failure was classified before the save; cancelling the save does not interrupt it.
			require.EqualValues(t, 1, n)
		})
	}
}
