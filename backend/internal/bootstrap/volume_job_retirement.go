// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original public-API retirement of removed centralized volume jobs.

package bootstrap

import (
	"context"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/florianfinn/docklet-next/types/scheduler"
)

// Retired jobs keep terminal audit history, but cannot schedule or resume work.
// This runs before coordinator activation, so no retired executor can race it.
func retireSystemVolumeJobs(ctx context.Context, coordinator *runs.Coordinator) error {
	records, err := coordinator.Records(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if !strings.HasPrefix(record.JobID, "system-backup:volumes:") {
			continue
		}
		// Centralized volume policies only scheduled on this host (environment 0).
		if record.EnvironmentID == "0" {
			if err := coordinator.Checkpoint(ctx, record.JobID, "", time.Time{}); err != nil {
				return err
			}
		}
		for _, run := range record.Runs {
			if run.Status.Terminal() {
				continue
			}
			if err := coordinator.UpdateRun(ctx, run, func(current *scheduler.Run) error {
				if current.Status.Terminal() {
					return nil
				}
				now := time.Now().UTC()
				current.Status = scheduler.Failed
				current.Outcome = scheduler.Outcome{Status: scheduler.Failed, Code: "runtime.interrupted", Message: "System-managed volume backups have been retired"}
				current.UpdatedAt = now
				current.FinishedAt = &now
				current.NextAttempt = nil
				current.Owner = ""
				return nil
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
