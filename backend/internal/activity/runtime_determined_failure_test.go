// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M13 closing-verification regressions.

package activity

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	dto "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"io"
	"testing"
	"time"
)

func TestRuntimeDeterminedFailureNotifiesWhenShutdownInterruptsItsSave(t *testing.T) {
	for _, kind := range []string{"deploy", "redeploy", "update", "restore", "policy"} {
		t.Run(kind, func(t *testing.T) {
			app, stop := context.WithCancel(t.Context())
			defer stop()
			var work context.Context
			f := b2Fixture(t, app, func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error {
				work = ctx
				return errors.New("controlled workflow failure")
			})
			injected := false
			require.NoError(t, f.store.svc.db.Callback().Create().Before("gorm:create").Register("test:late-stop", func(tx *gorm.DB) {
				if m, ok := tx.Statement.Dest.(*ActivityMessage); ok && m.Code == string(dto.CodeRuntimeFailed) {
					injected = true
					stop()
					select {
					case <-work.Done():
					case <-time.After(3 * time.Second):
						t.Error("work context did not cancel")
					}
				}
			}))
			c := f.command()
			switch kind {
			case "redeploy":
				c.Operation = RuntimeRedeployProject
			case "update":
				c = imageCommand(c, RuntimeUpdateProject)
			case "restore":
				c = imageCommand(c, RuntimeRestoreProject)
			case "policy":
				c, _ = policyCommand(t, f)
			}
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			settled, err := b2Wait(t, f, c, admitted.ActivityID)
			require.NoError(t, err)
			require.True(t, injected)
			require.ErrorIs(t, work.Err(), context.Canceled)
			var rows []notificationoutbox.Row
			require.NoError(t, f.store.svc.db.Find(&rows).Error)
			t.Logf("status=%s code=%s notices=%d", settled.Status, settled.Activity.MessageCode, len(rows))
			// The executor already determined the failure; shutdown while saving it is not an interruption.
			require.Len(t, rows, 1, "a determined failure notifies once")
			require.Equal(t, "deploy_failed", rows[0].Event)
		})
	}
}
