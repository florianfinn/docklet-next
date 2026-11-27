// Source: getarcaneapp/arcane/backend/pkg/scheduler/runs/actors.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; SQLite-only local persistence; M18 live scheduling, policy and contract foundation.

package runs

import (
	"context"
	"time"
)

// Stop rejects new submissions, cancels execution, and waits for workers until
// ctx expires. Unfinished claims remain persisted for startup reconciliation.
func (q *Coordinator) Stop(ctx context.Context) error {
	q.mu.Lock()
	q.stopping = true
	q.stopDeadline, _ = ctx.Deadline()
	if q.stopDeadline.IsZero() {
		q.stopDeadline = time.Now().Add(30 * time.Second)
	}
	q.enabled.Store(false)
	started := q.started
	if q.cancel != nil {
		q.cancel()
	}
	q.mu.Unlock()
	if !started {
		return nil
	}
	select {
	case <-q.done:
		settled := make(chan struct{})
		go func() { q.workers.Wait(); close(settled) }()
		select {
		case <-settled:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Successful work gets a detached write budget while remaining a tracked worker.
func (q *Coordinator) outcomeContextInternal(ctx context.Context) (context.Context, context.CancelFunc) {
	q.mu.Lock()
	deadline := q.stopDeadline
	q.mu.Unlock()
	budget := 30 * time.Second
	if !deadline.IsZero() {
		budget = min(budget, time.Until(deadline)-time.Second)
	}
	return context.WithTimeout(context.WithoutCancel(ctx), budget)
}
