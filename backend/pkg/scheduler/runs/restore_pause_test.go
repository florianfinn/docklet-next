// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original scheduler drain test for issue #32.

package runs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRestorePauseRejectsIntakeAndWaitsForWorker(t *testing.T) {
	q := &Coordinator{wake: make(chan struct{}, 1)}
	q.enabled.Store(true)
	work, admitted, release := q.trackRestoreWorkerInternal(t.Context())
	require.True(t, admitted)
	done := make(chan error, 1)
	go func() { done <- q.PauseAndDrain(t.Context()) }()
	select {
	case <-work.Done():
	case <-time.After(time.Second):
		t.Fatal("running job was not cancelled")
	}
	_, admitted, _ = q.trackRestoreWorkerInternal(t.Context())
	require.False(t, admitted)
	select {
	case <-done:
		t.Fatal("pause returned before local job completed")
	default:
	}
	release()
	require.NoError(t, <-done)
	q.Resume()
	_, admitted, release = q.trackRestoreWorkerInternal(t.Context())
	require.True(t, admitted)
	release()
}
