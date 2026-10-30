// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original bounded agent send regressions.

package backupnotify

import (
	"context"
	"errors"
	"testing"
	"time"

	dto "github.com/florianfinn/docklet-next/types/notification"
	"github.com/stretchr/testify/require"
)

// blockedSendInternal ignores its context, like the Edge tunnel send, until released or after 3 s.
func blockedSendInternal(release <-chan struct{}, sent chan<- struct{}) Dispatch {
	return func(context.Context, dto.DispatchRequest) error {
		select {
		case <-release:
		case <-time.After(3 * time.Second):
		}
		sent <- struct{}{}
		return nil
	}
}

func TestSendDetachedBoundsAProducerBehindABlockedTransport(t *testing.T) {
	release, sent := make(chan struct{}), make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	started := time.Now()
	err := SendDetached(ctx, 50*time.Millisecond, blockedSendInternal(release, sent), dto.DispatchRequest{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), 2*time.Second, "the producer waits only for the bound")
	close(release)
	select {
	case <-sent:
	case <-time.After(2 * time.Second):
		t.Fatal("the transport-owned send never finished")
	}
}

func TestSendDetachedReturnsTheTransportResult(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var seen error
	require.NoError(t, SendDetached(ctx, time.Second, func(c context.Context, _ dto.DispatchRequest) error {
		seen = c.Err()
		return nil
	}, dto.DispatchRequest{}))
	require.NoError(t, seen, "a cancelled run still sends with a live context")
	require.EqualError(t, SendDetached(t.Context(), time.Second, func(context.Context, dto.DispatchRequest) error {
		return errors.New("controlled failure")
	}, dto.DispatchRequest{}), "controlled failure")
	require.Error(t, SendDetached(t.Context(), time.Second, func(context.Context, dto.DispatchRequest) error {
		panic("controlled panic")
	}, dto.DispatchRequest{}))
}

func TestAgentBackupNoticeIsBoundedBehindABlockedTransport(t *testing.T) {
	release, sent := make(chan struct{}), make(chan struct{}, 1)
	n := Notifier{Agent: true, Dispatch: blockedSendInternal(release, sent), Wait: 50 * time.Millisecond}
	started := time.Now()
	n.Terminal(t.Context(), Run{ID: "run", Operation: "backup", Volume: "data", Status: "failed", Reason: "volume.backup.failed"})
	require.Less(t, time.Since(started), 2*time.Second, "a blocked transport must not hold the backup run")
	close(release)
	select {
	case <-sent:
	case <-time.After(2 * time.Second):
		t.Fatal("the notice was never sent")
	}
}
