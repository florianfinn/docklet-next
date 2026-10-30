// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package scheduler

import (
	"context"

	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"time"
)

type NotificationDeliverJob struct {
	Service interface {
		DeliverPending(context.Context, time.Time) error
	}
}

func (*NotificationDeliverJob) Name() string                    { return "notification-deliver" }
func (*NotificationDeliverJob) Schedule(context.Context) string { return "0 * * * * *" }
func (j *NotificationDeliverJob) Run(ctx context.Context) (st.Outcome, error) {
	err := j.Service.DeliverPending(ctx, time.Now().UTC())
	if err != nil {
		return st.Outcome{Status: st.Failed}, err
	}
	return st.Outcome{Status: st.Succeeded}, nil
}
func (*NotificationDeliverJob) Reconcile(context.Context, st.Run) (st.Outcome, error) {
	return st.Outcome{Status: st.Failed}, nil
}

// NotificationDeliveryWake submits work; all delivery effects stay owned by the job coordinator.
type NotificationDeliveryWake struct{ Scheduler st.JobScheduler }

func (*NotificationDeliveryWake) Name() string                 { return "notification-delivery-wake" }
func (*NotificationDeliveryWake) RunNow(context.Context) error { return nil }
func (w *NotificationDeliveryWake) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-notificationoutbox.Wakes():
			_, _ = w.Scheduler.Submit(ctx, st.Request{JobID: "notification-deliver", EnvironmentID: "0", Trigger: "event"})
		}
	}
}
