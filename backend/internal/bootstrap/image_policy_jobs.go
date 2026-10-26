// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Hub-only policy scheduler construction and registered-host lifecycle refresh.

package bootstrap

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler"
	"go.uber.org/fx"
	"log/slog"
)

func registerImagePolicyJobs(params registerJobsParams) error {
	params.JobSchedule.SetImagePolicies(params.Updater)
	if params.Config.AgentMode || params.Config.EdgeAgent {
		return nil
	}
	lifetime, cancel := context.WithCancel(params.AppCtx)
	refresh := func(context.Context) error {
		return scheduler.RefreshImagePolicyJobs(params.AppCtx, params.Scheduler, params.Updater)
	}
	params.Updater.ConfigureImagePolicies(params.Environment, refresh)
	params.Updater.ConfigureSystemImageHints(params.Environment, params.Version)
	if err := refresh(params.AppCtx); err != nil {
		cancel()
		return err
	}
	changed, unsubscribe := params.Environment.SubscribeRuntimeChanges()
	done := make(chan struct{})
	params.Lifecycle.Append(fx.Hook{OnStart: func(context.Context) error {
		go func() {
			defer close(done)
			for {
				select {
				case <-lifetime.Done():
					return
				case _, open := <-changed:
					if !open {
						return
					}
					// A failed refresh leaves existing jobs guarded by current registration;
					// the next environment change or schedule write retries registration.
					if err := refresh(lifetime); err != nil {
						slog.WarnContext(lifetime, "Failed to refresh hub image policy schedules")
					}
				}
			}
		}()
		return nil
	}, OnStop: func(ctx context.Context) error {
		cancel()
		unsubscribe()
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}})
	return nil
}
