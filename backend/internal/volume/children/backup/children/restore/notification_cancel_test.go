// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original cancelled-restore notification regressions through Execute.

package restore

import (
	"context"
	"errors"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/notification/backupnotify"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"github.com/stretchr/testify/require"
)

// failRestoreInternal makes one restore phase fail; cancel, when set, cancels the work first.
func failRestoreInternal(l *Lifecycle, phase string, cancel context.CancelFunc) {
	stop := func() {
		if cancel != nil {
			cancel()
		}
	}
	switch phase {
	case "writing":
		l.Effects.Write = func(context.Context, *Run, StartRequest) error { stop(); return errors.New("write stopped") }
	case "preparing":
		l.Effects.Prepare = func(context.Context, *Run, StartRequest) (func() error, error) {
			stop()
			return nil, errors.New("prepare stopped")
		}
	}
}

func TestCancelledRestoreThroughExecuteStaysSilent(t *testing.T) {
	for _, phase := range []string{"writing", "preparing"} {
		for _, cancelled := range []bool{true, false} {
			name := phase + "/failed"
			if cancelled {
				name = phase + "/cancelled"
			}
			t.Run(name+"/hub", func(t *testing.T) {
				l, r := lifecycleFixture(t)
				require.NoError(t, l.DB.AutoMigrate(&notificationoutbox.Row{}))
				l.Notify = backupnotify.Notifier{DB: l.DB}.Terminal
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if !cancelled {
					cancel = nil
				}
				failRestoreInternal(l, phase, cancel)
				require.Error(t, l.Execute(ctx, r, StartRequest{}))
				require.Equal(t, "failed", r.Status)
				var rows []notificationoutbox.Row
				require.NoError(t, l.DB.Find(&rows).Error)
				if cancelled {
					require.Empty(t, rows, "a cancelled restore stays silent")
				} else {
					require.Len(t, rows, 1, "a failed restore notifies once")
				}
			})
			t.Run(name+"/agent", func(t *testing.T) {
				l, r := lifecycleFixture(t)
				calls := 0
				l.Notify = backupnotify.Notifier{Agent: true, Dispatch: func(context.Context, dto.DispatchRequest) error {
					calls++
					return nil
				}}.Terminal
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if !cancelled {
					cancel = nil
				}
				failRestoreInternal(l, phase, cancel)
				require.Error(t, l.Execute(ctx, r, StartRequest{}))
				if cancelled {
					require.Zero(t, calls, "a cancelled restore dispatches nothing")
				} else {
					require.Equal(t, 1, calls, "a failed restore dispatches once")
				}
			})
		}
	}
}
