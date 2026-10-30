// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package bootstrap

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/autoheal"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type healSchedulerEffect struct {
	st.JobScheduler
	done chan error
}

func (s healSchedulerEffect) RescheduleJob(ctx context.Context, j st.Job) error {
	e := s.JobScheduler.RescheduleJob(ctx, j)
	s.done <- e
	return e
}
func TestTypedAutoHealDisableRemovesActualScheduledRegistration(t *testing.T) {
	db, e := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db")), database.MigrationOptions{})
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	s, e := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, s.Stop(context.Background())) })
	require.NoError(t, s.SetBoolSetting(t.Context(), "autoHealEnabled", true))
	runtime := francistest.New(t)
	coord := runs.New(runtime.Service(), time.UTC)
	require.NoError(t, coord.Register(runtime))
	admission := runs.NewAdmission(runtime.Service(), t.Name())
	require.NoError(t, admission.Register(runtime))
	francistest.Start(t, runtime)
	job, e := scheduler.NewAutoHealJob(nil, s, nil, nil, admission, scheduler.AutoHealOptions{DB: db})
	require.NoError(t, e)
	clock, e := scheduler.NewJobScheduler(t.Context(), coord, time.UTC)
	require.NoError(t, e)
	require.NoError(t, clock.RegisterJob(job))
	require.NoError(t, clock.StartScheduler(t.Context()))
	t.Cleanup(func() { require.NoError(t, clock.Stop(context.Background())) })
	state, ok := clock.GetJobRuntimeState(job.Name())
	require.True(t, ok)
	require.True(t, state.Scheduled)
	done := make(chan error, 1)
	lifecycle := fxtest.NewLifecycle(t)
	require.NoError(t, setupSettingsSubscriptionsInternal(settingsSubscriptionsParams{Lifecycle: lifecycle, LifecycleCtx: t.Context(), Config: &config.Config{}, Settings: s, Scheduler: healSchedulerEffect{clock, done}, AutoHeal: job}))
	t.Cleanup(func() { require.NoError(t, lifecycle.Stop(context.Background())) })
	_, api := humatest.New(t)
	api.UseMiddleware(func(c huma.Context, next func(huma.Context)) {
		next(huma.WithContext(c, context.WithValue(c.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	autoheal.New(s, nil, &config.Config{}).RegisterRoutes(api)
	resp := api.Put("/environments/0/auto-heal", strings.NewReader(`{"enabled":false,"maxRestarts":5,"windowMinutes":30,"excludedStacks":[],"excludedContainers":[]}`))
	require.Equal(t, 200, resp.Code, resp.Body.String())
	select {
	case e := <-done:
		require.NoError(t, e)
	case <-time.After(3 * time.Second):
		t.Fatal("typed update never reached scheduler")
	}
	state, ok = clock.GetJobRuntimeState(job.Name())
	require.True(t, ok)
	require.False(t, state.Scheduled)
}
