// Source: getarcaneapp/arcane/backend/pkg/scheduler/auto_update_job_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; replace retired-trigger tests with zero-effect ownership guards; functional cut: removed upstream analytics heartbeat; SQLite temp-file persistence fixture.

package scheduler

import (
	"context"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"testing"
)

type retiredApplier struct{ calls int }

func (f *retiredApplier) ApplyPending(context.Context, dto.Options) (*dto.Result, error) {
	f.calls++
	return &dto.Result{}, nil
}
func TestLegacyAutoUpdateJobCannotExecute(t *testing.T) {
	_, settings, _ := setupSchedulerStateServicesInternal(t)
	require.NoError(t, settings.SetBoolSetting(t.Context(), "autoUpdate", true))
	require.NoError(t, settings.SetBoolSetting(t.Context(), "pollingEnabled", true))
	fake := &retiredApplier{}
	job := &AutoUpdateJob{updaterService: fake, settingsService: settings}
	require.False(t, job.ShouldSchedule(t.Context()))
	for _, recovery := range []bool{false, true} {
		var out st.Outcome
		var err error
		if recovery {
			out, err = job.Reconcile(t.Context(), st.Run{JobID: "auto-update", AttemptCount: 2})
		} else {
			out, err = job.Run(t.Context())
		}
		require.NoError(t, err)
		require.Equal(t, st.Skipped, out.Status)
		require.Equal(t, "update.agent-local-job-disabled", out.Code)
	}
	require.Equal(t, "update.agent-local-job-disabled", imageDTO.Neutral(job.ValidateRetry(t.Context(), st.Run{}), "unexpected", 500).Problem.Code)
	require.Zero(t, fake.calls, "inherited apply must never be an independent effect path")
}
