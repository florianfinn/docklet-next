// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original dynamic-job reconciliation after Hub database replacement for issue #32.

package bootstrap

import (
	"context"
	"strings"

	schedulertypes "github.com/florianfinn/docklet-next/types/scheduler"
)

// Restore replaces policy rows, so dynamic registrations must be rebuilt
// from the replacement database before scheduler intake resumes.
func refreshDynamicJobsAfterRestore(ctx context.Context, params dynamicJobsParams) error {
	return rebuildDynamicJobsAfterRestore(ctx, params.Scheduler, func() error { return registerDynamicJobs(params) })
}

func rebuildDynamicJobsAfterRestore(ctx context.Context, scheduler schedulertypes.JobScheduler, register func() error) error {
	for _, job := range scheduler.ListRegisteredJobs() {
		name := job.Name()
		if strings.HasPrefix(name, "system-backup:") ||
			strings.HasPrefix(name, "volume-backup:") ||
			strings.HasPrefix(name, "environment-health:") {
			scheduler.RemoveJob(ctx, name)
		}
	}
	return register()
}
