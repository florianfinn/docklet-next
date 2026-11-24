// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: One hub observation job and target-owned window jobs; no inherited local watcher.

package scheduler

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/updater"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"strings"
	"sync"
)

// refreshWindows serialises refresh passes so a pass that read older rows cannot
// remove a window job that a concurrent policy save just installed.
var refreshWindows sync.Mutex

// RefreshUpdateWindows installs one observation job and the current scheduled
// policy windows. Every window re-reads its policy before admitting an effect.
func RefreshUpdateWindows(ctx context.Context, controller st.JobScheduler, owner *updater.UpdaterService) error {
	refreshWindows.Lock()
	defer refreshWindows.Unlock()
	rows, err := owner.ScheduledUpdatePolicies(ctx)
	if err != nil {
		return err
	}
	check := &st.GenericJob{JobName: "update-check", ScheduleFn: owner.UpdateCheckSchedule, RunFn: owner.RunUpdateCheck,
		ReconcileFn: func(ctx context.Context, run st.Run) (st.Outcome, error) { return owner.RunUpdateCheck(ctx) }}
	if err := controller.AddJob(ctx, check); err != nil {
		return err
	}
	// The static host job is manual-only. Its host comes from the durable run,
	// never from a dynamic job name or agent-local scheduler.
	hostCheck := &st.GenericJob{JobName: "update-check-host", ScheduleFn: func(context.Context) string { return "" }, ShouldRunFn: func(context.Context) bool { return false },
		RunFn: func(ctx context.Context) (st.Outcome, error) {
			run, ok := jobcontext.Run(ctx)
			if !ok || run.JobID != "update-check-host" {
				return st.Outcome{Status: st.Failed, Code: "update.invalid-target"}, nil
			}
			return owner.RunUpdateCheckHost(ctx, run.EnvironmentID)
		}, ReconcileFn: func(ctx context.Context, run st.Run) (st.Outcome, error) {
			return owner.RunUpdateCheckHost(ctx, run.EnvironmentID)
		}}
	if err := controller.AddJob(ctx, hostCheck); err != nil {
		return err
	}
	wanted := map[string]bool{}
	for _, row := range rows {
		if row.Window() == nil {
			return errors.New("scheduled policy window unavailable")
		}
		id, env := row.ID, row.EnvironmentID
		name := updater.UpdateWindowJobKey(env, row.TargetKind, row.TargetID)
		cron := row.Window().Cron()
		job := &st.GenericJob{JobName: name, ScheduleFn: func(context.Context) string { return cron },
			RunFn: func(ctx context.Context) (st.Outcome, error) { return owner.RunUpdateWindow(ctx, id) },
			ReconcileFn: func(ctx context.Context, run st.Run) (st.Outcome, error) {
				return owner.ReconcileImagePolicyHost(ctx, env, run)
			}}
		wanted[name] = true
		if err := controller.AddJob(ctx, job); err != nil {
			return err
		}
	}
	for _, job := range controller.ListRegisteredJobs() {
		if strings.HasPrefix(job.Name(), "update-window:") && !wanted[job.Name()] {
			controller.RemoveJob(ctx, job.Name())
		}
	}
	return nil
}
