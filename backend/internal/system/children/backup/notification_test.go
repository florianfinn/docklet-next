// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package backup

import (
	"context"
	"errors"
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

func TestLiveSystemBackupFailurePostSaveNotification(t *testing.T) {
	for _, kind := range []string{"manual", "scheduled", "save-failed", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			g, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
			require.NoError(t, err)
			db := &database.DB{DB: g}
			require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}))
			require.NoError(t, db.AutoMigrate(activity.Models()...))
			saved := false
			s := NewService(Dependencies{DB: db, Config: &config.Config{}, Activity: activity.NewActivityService(db, nil), Store: Store{SaveRun: func(context.Context, *dto.SystemBackupRun) error {
				saved = true
				if kind == "save-failed" {
					return errors.New("controlled save failure")
				}
				return nil
			}}})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if kind == "cancelled" {
				cancel()
			}
			require.Error(t, s.finishRun(ctx, &dto.SystemBackupRun{ID: kind, Trigger: kind}))
			require.True(t, saved)
			var rows []notificationoutbox.Row
			require.NoError(t, db.Find(&rows).Error)
			if kind == "manual" || kind == "scheduled" {
				require.Len(t, rows, 1)
			} else {
				require.Empty(t, rows)
			}
		})
	}
}
