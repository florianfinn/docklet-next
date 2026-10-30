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

func TestRestoreDeterminedFailureNotifiesWhenCancelledWhileSaved(t *testing.T) {
	for _, agent := range []bool{false, true} {
		for _, cancelled := range []bool{false, true} {
			t.Run(fmt.Sprintf("agent=%t/cancel=%t", agent, cancelled), func(t *testing.T) {
				l, r := lifecycleFixture(t)
				require.NoError(t, l.DB.AutoMigrate(&notificationoutbox.Row{}))
				calls := 0
				l.Notify = backupnotify.Notifier{DB: l.DB, Agent: agent, Dispatch: func(context.Context, nt.DispatchRequest) error { calls++; return nil }}.Terminal
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				l.Effects.Write = func(context.Context, *Run, StartRequest) error { return errors.New("controlled write failure") }
				injected := false
				require.NoError(t, l.DB.Callback().Update().Before("gorm:update").Register("test:terminal-save", func(tx *gorm.DB) {
					if row, ok := tx.Statement.Dest.(*Run); ok && row.FinishedAt != nil {
						injected = true
						if cancelled {
							cancel()
						}
					}
				}))
				require.Error(t, l.Execute(ctx, r, StartRequest{}))
				require.True(t, injected)
				var n int64
				require.NoError(t, l.DB.Model(&notificationoutbox.Row{}).Count(&n).Error)
				// The write failure determined the outcome before the save; cancelling the save does not
				// turn it into an interruption.
				if agent {
					require.Equal(t, 1, calls)
				} else {
					require.EqualValues(t, 1, n)
				}
			})
		}
	}
}

func TestRestoreEarlySitesAndPanicFollowTheWorkCancellation(t *testing.T) {
	for _, stage := range []string{"initial", "link", "panic"} {
		for _, agent := range []bool{false, true} {
			for _, cancelled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/agent=%t/cancel=%t", stage, agent, cancelled), func(t *testing.T) {
					l, r := lifecycleFixture(t)
					require.NoError(t, l.DB.AutoMigrate(&notificationoutbox.Row{}))
					calls := 0
					l.Notify = backupnotify.Notifier{DB: l.DB, Agent: agent, Dispatch: func(context.Context, nt.DispatchRequest) error { calls++; return nil }}.Terminal
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					if stage == "panic" {
						l.Effects.Prepare = func(context.Context, *Run, StartRequest) (func() error, error) {
							if cancelled {
								cancel()
							}
							panic("controlled restore panic")
						}
						require.Error(t, l.Execute(ctx, r, StartRequest{}))
					} else {
						l.Effects.Validate = func(context.Context, StartRequest) error { return nil }
						l.Effects.Admit = func(ctx context.Context, _, _ string) (context.Context, func(), error) { return ctx, func() {}, nil }
						if stage == "link" {
							require.NoError(t, l.DB.AutoMigrate(activity.Models()...))
							l.Activity = activity.NewActivityService(l.DB, nil)
						}
						injected := false
						require.NoError(t, l.DB.Callback().Update().Before("gorm:update").Register("test:early", func(tx *gorm.DB) {
							row, ok := tx.Statement.Dest.(*Run)
							if !ok || injected {
								return
							}
							if stage == "link" && row.ActivityID != "" && row.Status == "running" {
								injected = true
								if cancelled {
									require.True(t, l.Activity.RequestCancel(row.ActivityID))
								}
								tx.AddError(errors.New("controlled link failure"))
							} else if stage == "initial" && row.FinishedAt != nil {
								injected = true
								if cancelled {
									cancel()
								}
							}
						}))
						_, err := l.Start(ctx, StartRequest{EnvironmentID: "0", VolumeName: "data", Kind: "full", User: user.SystemUser})
						require.Error(t, err)
						require.True(t, injected)
					}
					var n int64
					require.NoError(t, l.DB.Model(&notificationoutbox.Row{}).Count(&n).Error)
					// The initial failure is determined before its save; link and panic cancel first.
					if cancelled && stage != "initial" {
						require.Zero(t, n)
						require.Zero(t, calls)
					} else if agent {
						require.Equal(t, 1, calls)
					} else {
						require.EqualValues(t, 1, n)
					}
				})
			}
		}
	}
}
