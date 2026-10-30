// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M13 closing-verification regressions.

package restore

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/notification/backupnotify"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	nt "github.com/florianfinn/docklet-next/types/notification"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestRestoreEarlyDeterminedFailureNotifiesWhenCancelledWhileSaved(t *testing.T) {
	for _, stage := range []string{"initial", "link"} {
		for _, agent := range []bool{false, true} {
			for _, late := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/agent=%t/late=%t", stage, agent, late), func(t *testing.T) {
					l, _ := lifecycleFixture(t)
					require.NoError(t, l.DB.AutoMigrate(&notificationoutbox.Row{}))
					calls := 0
					l.Notify = backupnotify.Notifier{DB: l.DB, Agent: agent, Dispatch: func(ctx context.Context, _ nt.DispatchRequest) error {
						require.NoError(t, ctx.Err())
						calls++
						return nil
					}}.Terminal
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					l.Effects.Validate = func(context.Context, StartRequest) error { return nil }
					l.Effects.Admit = func(ctx context.Context, _, _ string) (context.Context, func(), error) { return ctx, func() {}, nil }
					if stage == "link" {
						require.NoError(t, l.DB.AutoMigrate(activity.Models()...))
						l.Activity = activity.NewActivityService(l.DB, nil)
					}
					linked, saved := false, false
					require.NoError(t, l.DB.Callback().Update().Before("gorm:update").Register("test:early-terminal", func(tx *gorm.DB) {
						row, ok := tx.Statement.Dest.(*Run)
						if !ok {
							return
						}
						if stage == "link" && row.ActivityID != "" && row.Status == "running" && !linked {
							linked = true
							tx.AddError(errors.New("controlled link failure"))
							return
						}
						if row.FinishedAt != nil {
							saved = true
							if late {
								cancel()
							}
						}
					}))
					_, err := l.Start(ctx, StartRequest{EnvironmentID: "0", VolumeName: "data", Kind: "full", User: user.SystemUser})
					require.Error(t, err)
					require.True(t, saved)
					if stage == "link" {
						require.True(t, linked)
					}
					var rows []notificationoutbox.Row
					require.NoError(t, l.DB.Find(&rows).Error)
					if agent {
						require.Equal(t, 1, calls, "determined early failure dispatches despite cancellation inside terminal save")
					} else {
						require.Len(t, rows, 1, "determined early failure enqueues despite cancellation inside terminal save")
					}
				})
			}
		}
	}
}
