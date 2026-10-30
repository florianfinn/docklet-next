// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package scheduler

import (
	"context"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type heldNotificationDelivery struct{ entered, cancelled, release chan struct{} }

func (d *heldNotificationDelivery) DeliverPending(ctx context.Context, _ time.Time) error {
	close(d.entered)
	<-ctx.Done()
	close(d.cancelled)
	<-d.release
	return ctx.Err()
}
func TestNotificationDeliveryRemainsOwnedDuringDrain(t *testing.T) {
	delivery := &heldNotificationDelivery{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	job := &NotificationDeliverJob{Service: delivery}
	q, _ := newTestCoordinatorInternal(t, t.Context(), time.UTC, func(ctx context.Context, _ st.Run) (st.Outcome, error) { return job.Run(ctx) })
	_, err := q.Submit(t.Context(), st.Request{JobID: job.Name(), Trigger: "event"})
	require.NoError(t, err)
	<-delivery.entered
	drained := make(chan error, 1)
	go func() { drained <- q.PauseAndDrain(t.Context()) }()
	<-delivery.cancelled
	select {
	case err := <-drained:
		close(delivery.release)
		t.Fatalf("drain returned before delivery joined: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(delivery.release)
	require.NoError(t, <-drained)
	_, err = q.Submit(t.Context(), st.Request{JobID: job.Name(), Trigger: "event"})
	require.Error(t, err)
}
