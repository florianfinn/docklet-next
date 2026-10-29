// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package scheduler

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"time"
)

type HostAvailabilityJob struct {
	Service *notification.NotificationService
}

func (*HostAvailabilityJob) Name() string                    { return "host-availability" }
func (*HostAvailabilityJob) Schedule(context.Context) string { return "0 * * * * *" }
func (j *HostAvailabilityJob) Run(ctx context.Context) (st.Outcome, error) {
	err := j.Service.ObserveAvailability(ctx, time.Now().UTC())
	return st.Outcome{Status: st.Succeeded}, err
}
func (*HostAvailabilityJob) Reconcile(context.Context, st.Run) (st.Outcome, error) {
	return st.Outcome{Status: st.Skipped}, nil
}
