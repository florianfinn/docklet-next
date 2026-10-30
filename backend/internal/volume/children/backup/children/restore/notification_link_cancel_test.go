// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M13 closing-verification regressions.

package restore

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/notification/backupnotify"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestRestoreCancelledDuringActivityLinkFailureStaysSilent(t *testing.T) {
	l, _ := lifecycleFixture(t)
	require.NoError(t, l.DB.AutoMigrate(activity.Models()...))
	require.NoError(t, l.DB.AutoMigrate(&notificationoutbox.Row{}))
	l.Activity = activity.NewActivityService(l.DB, nil)
	l.Notify = backupnotify.Notifier{DB: l.DB}.Terminal
	l.Effects.Validate = func(context.Context, StartRequest) error { return nil }
	l.Effects.Admit = func(ctx context.Context, _, _ string) (context.Context, func(), error) { return ctx, func() {}, nil }
	injected := false
	require.NoError(t, l.DB.Callback().Update().Before("gorm:update").Register("test:link-failure", func(tx *gorm.DB) {
		row, ok := tx.Statement.Dest.(*Run)
		if ok && row.ActivityID != "" && row.Status == "running" && !injected {
			injected = true
			require.True(t, l.Activity.RequestCancel(row.ActivityID))
			tx.AddError(errors.New("controlled link failure"))
		}
	}))
	_, err := l.Start(t.Context(), StartRequest{EnvironmentID: "0", VolumeName: "data", Kind: "full", User: user.SystemUser})
	require.Error(t, err)
	require.True(t, injected)
	var rows []notificationoutbox.Row
	require.NoError(t, l.DB.Find(&rows).Error)
	require.Empty(t, rows, "cancellation of work context must be retained at early terminal save")
}
