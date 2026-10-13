// Source: getarcaneapp/arcane/backend/pkg/scheduler/upgrade_log_cleanup_job.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package scheduler

import (
	"context"
	"log/slog"
	"time"

	schedulertypes "github.com/florianfinn/docklet-next/types/scheduler"

	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
)

const UpgradeLogCleanupJobName = "upgrade-log-cleanup"

// NewUpgradeLogCleanupJob creates the hourly local upgrade log cleanup job.
func NewUpgradeLogCleanupJob(service *system.SystemService) schedulertypes.Job {
	return &schedulertypes.GenericJob{
		JobName:    UpgradeLogCleanupJobName,
		ScheduleFn: func(context.Context) string { return "0 15 * * * *" },
		RunFn: func(ctx context.Context) (schedulertypes.Outcome, error) {
			removed, err := service.PruneUpgradeLogs(ctx, libarcane.UpgradeLogDirectory, time.Now())
			if err != nil {
				slog.ErrorContext(ctx, "Failed to clean up upgrade logs", "jobName", UpgradeLogCleanupJobName, "removed", removed, "error", err)
				return schedulertypes.Outcome{}, err
			}
			if removed > 0 {
				slog.InfoContext(ctx, "Removed expired upgrade logs", "jobName", UpgradeLogCleanupJobName, "removed", removed)
			}
			return schedulertypes.Outcome{Status: schedulertypes.Succeeded}, nil
		},
	}
}
