// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original restore cancellation boundary tests for issue #32.

package activity

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRestoreCancellationBoundary(t *testing.T) {
	work, cancel := context.WithCancelCause(t.Context())
	svc := &ActivityService{running: map[string]context.CancelCauseFunc{"restore": cancel}}
	require.NoError(t, svc.SealCancellation("restore"))
	require.False(t, svc.RequestCancel("restore"))
	require.NoError(t, work.Err())
	svc.releaseCancelInternal("restore")
}

func TestRestoreDrainWaitsForOtherActivityRelease(t *testing.T) {
	work, cancel := context.WithCancelCause(t.Context())
	svc := &ActivityService{running: map[string]context.CancelCauseFunc{"other": cancel}}
	done := make(chan error, 1)
	go func() { done <- svc.CancelAndDrain(t.Context(), "restore") }()
	select {
	case <-work.Done():
	case <-time.After(time.Second):
		t.Fatal("worker was not cancelled")
	}
	select {
	case <-done:
		t.Fatal("drain returned before worker released its activity")
	default:
	}
	svc.releaseCancelInternal("other")
	require.NoError(t, <-done)
}
