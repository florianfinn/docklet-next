// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package job

import (
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	scanScheduler "github.com/florianfinn/docklet-next/backend/pkg/scheduler"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/florianfinn/docklet-next/types/jobschedule"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
)

func TestAutoHealScheduledRunHasNoEmptyActivity(t *testing.T) {
	s, _, _ := newJobActivityTestServiceInternal(t)
	for _, trigger := range []string{"scheduled", "recovery", "manual"} {
		r, e := s.runs.Submit(t.Context(), scheduler.Request{JobID: "auto-heal", EnvironmentID: "0", Trigger: trigger})
		require.NoError(t, e)
		require.Equal(t, trigger == "manual", r.ActivityID != "")
	}
}

func TestAutoHealIntervalChangesWhileDisabled(t *testing.T) {
	db := setupSettingsTestDBInternal(t)
	cfg, err := newSettingsServiceForTestInternal(t, t.Context(), db)
	require.NoError(t, err)
	svc := newJobServiceForTestInternal(t, db, cfg, &config.Config{})
	heal, err := scanScheduler.NewAutoHealJob(nil, cfg, nil, nil, &runs.Admission{})
	require.NoError(t, err)
	controller, err := scanScheduler.NewJobScheduler(t.Context(), svc.runs, time.UTC)
	require.NoError(t, err)
	svc.SetScheduler(t.Context(), controller)
	require.NoError(t, controller.RegisterJob(heal))
	require.NoError(t, cfg.SetBoolSetting(t.Context(), "autoHealEnabled", false))
	interval := "0 */7 * * * *"
	_, err = svc.UpdateJobSchedules(t.Context(), jobschedule.Update{AutoHealInterval: &interval})
	require.NoError(t, err)
	require.Equal(t, interval, svc.GetJobSchedules(t.Context()).AutoHealInterval)
}
