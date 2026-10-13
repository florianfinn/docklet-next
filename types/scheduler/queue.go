// Source: getarcaneapp/arcane/types/scheduler/queue.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package scheduler

import (
	"context"
	"time"
)

// RunObserver projects persisted runs into operator-facing activity records.
// ActivityID is persisted before projection and is empty for runs that should stay quiet.
type RunObserver interface {
	ActivityID(run Run) string
	SyncRunActivity(ctx context.Context, run Run) error
}

// QueueRecord holds atomic admission, claims, and checkpoints for one job and target.
type QueueRecord struct {
	Generation     uint64         `json:"-"`
	Sequence       uint64         `json:"-"`
	JobID          string         `json:"jobId"`
	EnvironmentID  string         `json:"environmentId"`
	Schedule       string         `json:"schedule"`
	LastEnqueuedAt time.Time      `json:"lastEnqueuedAt"`
	NextRun        time.Time      `json:"nextRun"`
	Runs           []Run          `json:"runs"`
	Receipts       map[string]Run `json:"receipts,omitempty"`
}
