// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original stale-policy scheduler regression test for issue #32.

package bootstrap

import (
	"context"
	"testing"

	schedulertypes "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
)

type restoreJobScheduler struct {
	schedulertypes.JobScheduler
	jobs map[string]schedulertypes.Job
}

func (s *restoreJobScheduler) ListRegisteredJobs() []schedulertypes.Job {
	jobs := make([]schedulertypes.Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

func (s *restoreJobScheduler) RemoveJob(_ context.Context, name string) { delete(s.jobs, name) }

func (s *restoreJobScheduler) AddJob(_ context.Context, job schedulertypes.Job) error {
	s.jobs[job.Name()] = job
	return nil
}

func TestRebuildDynamicJobsAfterRestoreReplacesStalePolicy(t *testing.T) {
	scheduler := &restoreJobScheduler{jobs: map[string]schedulertypes.Job{}}
	for _, name := range []string{"system-backup:stale", "volume-backup:old", "environment-health:removed", "static-cleanup"} {
		scheduler.jobs[name] = &schedulertypes.GenericJob{JobName: name}
	}
	require.NoError(t, rebuildDynamicJobsAfterRestore(t.Context(), scheduler, func() error {
		for _, name := range []string{"system-backup:restored", "volume-backup:new", "environment-health:current"} {
			require.NoError(t, scheduler.AddJob(t.Context(), &schedulertypes.GenericJob{JobName: name}))
		}
		return nil
	}))
	names := make([]string, 0, len(scheduler.jobs))
	for name := range scheduler.jobs {
		names = append(names, name)
	}
	require.ElementsMatch(t, []string{"system-backup:restored", "volume-backup:new", "environment-health:current", "static-cleanup"}, names)
}
