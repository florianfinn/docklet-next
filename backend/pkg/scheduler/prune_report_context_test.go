// Source: docklet-next/issues/678
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original regression proofs.

package scheduler

import (
	"context"
	"errors"
	scheduler "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/stretchr/testify/require"
	"testing"
)

type cancelledPrune struct {
	cancel  context.CancelFunc
	failure error
}

func (p cancelledPrune) PruneScheduled(context.Context, string, system.PruneAllRequest) (*system.PruneAllResult, bool, error) {
	p.cancel()
	return &system.PruneAllResult{Errors: []string{"failed"}}, true, p.failure
}
func (p cancelledPrune) ReconcileScheduledPrune(context.Context, scheduler.Run) (scheduler.Outcome, error) {
	return scheduler.Outcome{}, nil
}

type reportProbe struct {
	called bool
	t      *testing.T
}

func (p *reportProbe) SendPruneReportNotification(ctx context.Context, _ *system.PruneAllResult) error {
	p.called = true
	require.NoError(p.t, ctx.Err())
	_, ok := ctx.Deadline()
	require.True(p.t, ok)
	return nil
}
func TestPruneReportSurvivesCancellationAndError(t *testing.T) {
	for _, failure := range []error{nil, errors.New("prune failed")} {
		settings := setupScheduledPruneSettingsServiceInternal(t)
		require.NoError(t, settings.UpdateSetting(t.Context(), "scheduledPruneEnabled", "true"))
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		probe := &reportProbe{t: t}
		job := &ScheduledPruneJob{systemService: cancelledPrune{cancel, failure}, settingsService: settings, notificationService: probe}
		_, err := job.Run(ctx)
		if failure != nil {
			require.ErrorIs(t, err, failure)
		} else {
			require.NoError(t, err)
		}
		require.True(t, probe.called)
	}
}

type failedPruneBeforePlan struct{}

func (failedPruneBeforePlan) PruneScheduled(context.Context, string, system.PruneAllRequest) (*system.PruneAllResult, bool, error) {
	return nil, false, context.Canceled
}
func (failedPruneBeforePlan) ReconcileScheduledPrune(context.Context, scheduler.Run) (scheduler.Outcome, error) {
	return scheduler.Outcome{}, nil
}
func TestPruneReportSentForFailureBeforePlan(t *testing.T) {
	settings := setupScheduledPruneSettingsServiceInternal(t)
	require.NoError(t, settings.UpdateSetting(t.Context(), "scheduledPruneEnabled", "true"))
	probe := &reportProbe{t: t}
	job := &ScheduledPruneJob{systemService: failedPruneBeforePlan{}, settingsService: settings, notificationService: probe}
	_, err := job.Run(t.Context())
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, probe.called)
}
