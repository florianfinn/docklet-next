// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original repeated-start proof for removed centralized scheduler jobs.

package bootstrap

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/job"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
)

func TestStartupRetiresSystemVolumeJobsIdempotentlyAndRetainsHistory(t *testing.T) {
	cfg := &config.Config{DatabaseURL: sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db")), AgentMode: true}
	db, err := database.InitializeHub(t.Context(), cfg.DatabaseURL, database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	settingsService, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, settingsService.Stop(context.WithoutCancel(t.Context()))) })
	runtime := francistest.New(t)
	q := runs.New(runtime.Service(), time.UTC)
	require.NoError(t, q.Register(runtime))
	francistest.Start(t, runtime)
	jobs := job.NewJobService(db, settingsService, cfg, q, nil, nil, nil)
	clock, err := scheduler.NewJobScheduler(t.Context(), q, time.UTC)
	require.NoError(t, err)
	jobs.SetScheduler(t.Context(), clock)
	t.Cleanup(func() { require.NoError(t, clock.Stop(context.WithoutCancel(t.Context()))) })
	for _, id := range []string{"system-backup:volumes:daily", "system-backup:volumes:weekly"} {
		require.NoError(t, q.Checkpoint(t.Context(), id, "0 0 0 * * *", time.Now().Add(-time.Hour)))
		for _, status := range []st.RunStatus{st.Queued, st.Waiting, st.Running, st.Retrying, st.NeedsAttention, st.Succeeded} {
			run, err := q.Submit(t.Context(), st.Request{JobID: id})
			require.NoError(t, err)
			require.NoError(t, q.UpdateRun(t.Context(), run, func(current *st.Run) error { current.Status = status; current.Outcome.Status = status; return nil }))
		}
	}
	// An unrelated Hub archive job retains its schedule and pending work.
	require.NoError(t, q.Checkpoint(t.Context(), "system-backup:hub-daily", "0 0 0 * * *", time.Now().Add(time.Hour)))
	unrelated, err := q.Submit(t.Context(), st.Request{JobID: "system-backup:hub-daily"})
	require.NoError(t, err)
	for start := range 2 {
		require.NoError(t, reconcileCoordinatorStartupInternal(t.Context(), jobs, nil, nil, nil, nil))
		records, err := q.Records(t.Context())
		require.NoError(t, err)
		for _, record := range records {
			if !strings.HasPrefix(record.JobID, "system-backup:volumes:") {
				continue
			}
			require.Empty(t, record.Schedule, "start %d", start)
			require.True(t, record.NextRun.IsZero(), "start %d", start)
			require.Len(t, record.Runs, 6)
			var successes int
			for _, run := range record.Runs {
				require.True(t, run.Status.Terminal(), "start %d: %s", start, run.Status)
				if run.Status == st.Succeeded {
					successes++
					continue
				}
				require.Equal(t, st.Failed, run.Status)
				require.Equal(t, "runtime.interrupted", run.Outcome.Code)
				require.NotNil(t, run.FinishedAt)
				require.Nil(t, run.NextAttempt)
			}
			require.Equal(t, 1, successes, "terminal history must remain unchanged")
			_, visible := clock.GetJobRuntimeState(record.JobID)
			require.False(t, visible, "retired job must have no next-run listing")
		}
		listing, err := jobs.ListJobs(t.Context())
		require.NoError(t, err)
		for _, row := range listing.Jobs {
			require.False(t, strings.HasPrefix(row.ID, "system-backup:volumes:"))
		}
		current, err := q.Get(t.Context(), "0", unrelated.JobID, unrelated.ID)
		require.NoError(t, err)
		require.Equal(t, st.Queued, current.Status)
		state, err := q.ScheduleState(t.Context(), unrelated.JobID)
		require.NoError(t, err)
		require.NotEmpty(t, state.Schedule)
		require.False(t, state.NextRun.IsZero())
	}
}
