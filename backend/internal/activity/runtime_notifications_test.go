// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package activity

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	dto "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestDeployNotificationSharesTerminalTransaction(t *testing.T) {
	for _, mode := range []string{"failed", "startup", "agent", "cancelled", "outbox-fault"} {
		t.Run(mode, func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			if mode == "agent" {
				t.Setenv("AGENT_MODE", "true")
			}
			if mode == "outbox-fault" {
				require.NoError(t, f.store.svc.db.Exec(`CREATE TRIGGER notification_fault BEFORE INSERT ON notification_outbox BEGIN SELECT RAISE(ABORT,'outbox fault'); END`).Error)
			}
			before := runtimeBoundarySnapshot(t, f.store)
			result := runtimeLocalResultInternal{state: "failed", code: dto.CodeRuntimeFailed, message: "Runtime failed", settledAt: f.store.now, silentNotification: mode == "startup"}
			if mode == "cancelled" {
				result.state = "cancelled"
				result.code = dto.CodeRuntimeCancelled
			}
			ctx := context.WithValue(t.Context(), gateRunContextKey{}, admitted.ActivityID)
			err = f.service.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
				_, _, err := runtimeCompleteTxInternal(tx, now, b1Identity(t, c, admitted.ActivityID), result)
				return err
			})
			if mode == "outbox-fault" {
				require.Error(t, err)
				require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			} else {
				require.NoError(t, err)
			}
			var rows []notificationoutbox.Row
			require.NoError(t, f.store.svc.db.Find(&rows).Error)
			if mode == "failed" {
				require.Len(t, rows, 1)
				require.Equal(t, "deploy:0:"+admitted.ActivityID, rows[0].DedupKey)
			} else {
				require.Empty(t, rows)
			}
		})
	}
}
func TestDeployFailureOperationFilterAndIdempotentReplay(t *testing.T) {
	f := b1Fixture(t)
	for _, op := range []string{"deploy-project", "redeploy-project", "update-project", "restore-project", "restart-project"} {
		row := &RuntimeReceipt{RunID: op, EnvironmentID: "0", OperationID: &op, TargetKind: new("stack"), TargetName: new("Example")}
		for range 2 {
			require.NoError(t, f.store.svc.db.Transaction(func(tx *gorm.DB) error { return runtimeNotifyFailureTx(tx, f.store.now, row, "runtime.failed", false) }))
		}
	}
	var rows []notificationoutbox.Row
	require.NoError(t, f.store.svc.db.Find(&rows).Error)
	require.Len(t, rows, 4)
}

func TestRemoteDeployApplyAndDispositionAreAtomicAndDeduplicated(t *testing.T) {
	for _, site := range []string{"direct", "edge", "disposition"} {
		for _, fault := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fault=%t", site, fault), func(t *testing.T) {
				p := &remoteTransportProbe{}
				f := remoteFixture(t, p, RuntimeReconcileSchedule{})
				p.execute = func(_ context.Context, e dto.RuntimeExecutionEnvelope) (dto.RuntimeExecutionOutcome, error) {
					return remoteAnswer(e.Identity, f.store.now, dto.StatusRunning), nil
				}
				f.start(t)
				before := f.receipt(t)
				if fault {
					require.NoError(t, f.store.svc.db.Exec(`CREATE TRIGGER notification_fault BEFORE INSERT ON notification_outbox BEGIN SELECT RAISE(ABORT,'controlled failure'); END`).Error)
				}
				apply := func() error {
					if site == "disposition" {
						r := f.receipt(t)
						return f.store.svc.db.Transaction(func(tx *gorm.DB) error {
							_, _, e := runtimeHubDispositionTxInternal(tx, f.store.now, &r, runtimeDispositionInternal{Disposition: "interrupted", By: "operator", Reason: "runtime.failed"})
							return e
						})
					}
					answer := remoteAnswer(runtimeWireIdentityInternal(&before), f.store.now, dto.StatusSuccess)
					answer.Detail.Activity.Status = dto.StatusFailed
					answer.Detail.Activity.MessageCode = string(dto.CodeRuntimeFailed)
					_, e := f.service.applyRemoteInternal(t.Context(), f.ref, answer, true)
					return e
				}
				e := apply()
				if fault {
					require.Error(t, e)
					require.Equal(t, before, f.receipt(t))
					require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER notification_fault").Error)
					require.NoError(t, apply())
				} else {
					require.NoError(t, e)
				}
				require.NoError(t, apply())
				var rows []notificationoutbox.Row
				require.NoError(t, f.store.svc.db.Find(&rows).Error)
				require.Len(t, rows, 1)
			})
		}
	}
}
