// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M13 closing-verification regressions.

package restore

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/notification/backupnotify"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRestoreCancelledDuringTerminalEffectsStaysSilent(t *testing.T) {
	for _, stage := range []string{"restart-after-write", "restart-before-write", "cleanup-new"} {
		for _, agent := range []bool{false, true} {
			name := stage + "/hub"
			if agent {
				name = stage + "/agent"
			}
			t.Run(name, func(t *testing.T) {
				l, r := lifecycleFixture(t)
				require.NoError(t, l.DB.AutoMigrate(&notificationoutbox.Row{}))
				calls := 0
				l.Notify = backupnotify.Notifier{DB: l.DB, Agent: agent, Dispatch: func(context.Context, dto.DispatchRequest) error { calls++; return nil }}.Terminal
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				switch stage {
				case "restart-after-write":
					l.Effects.Restart = func(context.Context, []volume.StoppedContainer, user.Actor) error {
						cancel()
						return errors.New("restart failed")
					}
				case "restart-before-write":
					l.Effects.Prepare = func(context.Context, *Run, StartRequest) (func() error, error) {
						return nil, errors.New("prepare failed")
					}
					l.Effects.Restart = func(context.Context, []volume.StoppedContainer, user.Actor) error { cancel(); return nil }
				case "cleanup-new":
					r.Mode = "new"
					r.NewVolumeName = "restored"
					l.Effects.Write = func(context.Context, *Run, StartRequest) error { return errors.New("write failed") }
					l.Effects.CleanupNew = func(context.Context, *Run) error { cancel(); return nil }
				}
				_ = l.Execute(ctx, r, StartRequest{})
				require.ErrorIs(t, ctx.Err(), context.Canceled)
				var rows []notificationoutbox.Row
				require.NoError(t, l.DB.Find(&rows).Error)
				t.Logf("status=%s reason=%s rows=%d calls=%d", r.Status, r.ErrorCode, len(rows), calls)
				require.Empty(t, rows, "cancelled restore must remain silent through terminal effects")
				require.Zero(t, calls, "cancelled restore must never dispatch")
			})
		}
	}
}
