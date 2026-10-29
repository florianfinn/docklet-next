// Source: getarcaneapp/arcane/backend/pkg/scheduler/auto_heal_job_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed upstream analytics heartbeat; Durable restart, final eligibility and real scheduler recovery evidence for issue #296.

package scheduler

import (
	"bytes"
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/autoheal"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/google/uuid"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func autoHealFixture(t *testing.T) (*AutoHealJob, *database.DB) {
	t.Helper()
	db, s, _ := setupSchedulerStateServicesInternal(t)
	require.NoError(t, db.AutoMigrate(activity.Models()...))
	require.NoError(t, db.AutoMigrate(&autoheal.Restart{}, &event.Event{}, &notificationoutbox.Row{}))
	pool, err := db.DB.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	require.NoError(t, s.SetBoolSetting(t.Context(), "autoHealEnabled", true))
	j, err := NewAutoHealJob(nil, s, nil, nil, newTestAdmissionGateInternal(t), AutoHealOptions{DB: db, Busy: func(context.Context, container.InspectResponse) (bool, error) { return false, nil }})
	require.NoError(t, err)
	j.getDockerClient = func() (*client.Client, error) { return nil, nil }
	j.getSelfContainerID = func() (string, error) { return "", nil }
	j.listContainers = func(context.Context, *client.Client) ([]container.Summary, error) {
		return []container.Summary{{ID: "container", Names: []string{"/web"}}}, nil
	}
	j.inspectContainer = func(_ context.Context, _ *client.Client, id string) (container.InspectResponse, error) {
		return healInspect(id), nil
	}
	j.restartContainer = func(context.Context, *client.Client, string) error { return nil }
	return j, db
}
func healInspect(id string) container.InspectResponse {
	return container.InspectResponse{ID: id, Name: "/web", Config: &container.Config{Labels: map[string]string{"com.docker.compose.project": "app"}, Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}}, State: &container.State{Running: true, Health: &container.Health{Status: container.Unhealthy}, StartedAt: "before"}}
}
func TestAutoHealFinalEligibilityUsesCurrentIdentitySettingsAndHealth(t *testing.T) {
	for _, kind := range []string{"stopped", "restarting", "paused", "healthy", "replaced", "no-healthcheck", "excluded-stack", "excluded-name", "disabled", "busy", "read-error"} {
		t.Run(kind, func(t *testing.T) {
			j, db := autoHealFixture(t)
			var calls atomic.Int32
			j.restartContainer = func(context.Context, *client.Client, string) error { calls.Add(1); return nil }
			j.inspectContainer = func(context.Context, *client.Client, string) (container.InspectResponse, error) {
				v := healInspect("container")
				switch kind {
				case "stopped":
					v.State.Running = false
				case "restarting":
					v.State.Restarting = true
				case "paused":
					v.State.Paused = true
				case "healthy":
					v.State.Health.Status = container.Healthy
				case "replaced":
					v.ID = "replacement"
				case "no-healthcheck":
					v.Config.Healthcheck = nil
				case "excluded-stack":
					require.NoError(t, j.settingsService.SetStringSetting(t.Context(), "autoHealExcludedStacks", "app"))
				case "excluded-name":
					require.NoError(t, j.settingsService.SetStringSetting(t.Context(), "autoHealExcludedContainers", "web"))
				case "disabled":
					require.NoError(t, j.settingsService.SetBoolSetting(t.Context(), "autoHealEnabled", false))
				}
				return v, nil
			}
			if kind == "busy" {
				j.busy = func(context.Context, container.InspectResponse) (bool, error) { return true, nil }
			}
			if kind == "read-error" {
				j.busy = func(context.Context, container.InspectResponse) (bool, error) {
					return false, errors.New("read failed")
				}
			}
			_, err := j.Run(t.Context())
			require.NoError(t, err)
			require.Zero(t, calls.Load())
			var n int64
			require.NoError(t, db.Model(&autoheal.Restart{}).Count(&n).Error)
			require.Zero(t, n)
		})
	}
}
func TestAutoHealFailedRestartIsDurableAndProjected(t *testing.T) {
	j, db := autoHealFixture(t)
	j.restartContainer = func(context.Context, *client.Client, string) error { return errors.New("controlled Docker failure") }
	outcome, err := j.Run(t.Context())
	require.NoError(t, err)
	require.Equal(t, st.Partial, outcome.Status)
	var row autoheal.Restart
	require.NoError(t, db.First(&row).Error)
	require.Equal(t, "restart-failed", row.Outcome)
	require.NotNil(t, row.ProjectedAt)
	var a activity.Activity
	require.NoError(t, db.First(&a).Error)
	require.Equal(t, "autoheal.restart-failed", a.MessageCode)
	var n int64
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.EqualValues(t, 1, n)
}
func TestAutoHealRecoveryClosesReservationCheckpointGapThroughScheduler(t *testing.T) {
	for _, effect := range []string{"before-docker", "after-docker", "still-restarting"} {
		t.Run(effect, func(t *testing.T) {
			j, db := autoHealFixture(t)
			runtime := francistest.New(t)
			q := runs.New(runtime.Service(), time.UTC)
			require.NoError(t, q.Register(runtime))
			francistest.Start(t, runtime)
			run, err := q.Submit(t.Context(), st.Request{JobID: j.Name(), RunID: uuid.NewString(), Trigger: "scheduled"})
			require.NoError(t, err)
			require.NoError(t, q.UpdateRun(t.Context(), run, func(r *st.Run) error { r.Status = st.Running; return nil }))
			var calls atomic.Int32
			j.restartContainer = func(context.Context, *client.Client, string) error { calls.Add(1); return nil }
			// Freeze the real plan, then fail its first target checkpoint after the reservation committed.
			ctx := jobcontext.WithExecution(t.Context(), run, func(target st.TargetOutcome) error {
				if target.ResourceType == "container" {
					return errors.New("checkpoint unavailable")
				}
				return q.UpdateRun(t.Context(), run, func(r *st.Run) error { r.Outcome.Targets = append(r.Outcome.Targets, target); return nil })
			})
			_, err = j.Run(ctx)
			require.Error(t, err)
			require.Zero(t, calls.Load())
			var reserved autoheal.Restart
			require.NoError(t, db.First(&reserved).Error)
			require.Equal(t, "attempting", reserved.Outcome)
			require.Equal(t, run.ID, reserved.RunID)
			j.inspectContainer = func(context.Context, *client.Client, string) (container.InspectResponse, error) {
				v := healInspect("container")
				if effect != "before-docker" {
					v.State.StartedAt = "after"
				}
				if effect == "still-restarting" {
					v.State.Restarting = true
				}
				v.State.Health.Status = container.Healthy
				return v, nil
			}
			recovered := make(chan st.Outcome, 1)
			q.SetExecutor(func(context.Context, st.Run) (st.Outcome, error) {
				t.Error("interrupted run replayed through ordinary execution")
				return st.Outcome{}, nil
			}, func(ctx context.Context, r st.Run) (st.Outcome, error) {
				o, e := j.Reconcile(ctx, r)
				recovered <- o
				return o, e
			})
			require.NoError(t, q.Start(t.Context(), t.Context()))
			q.Activate()
			defer q.Stop(context.WithoutCancel(t.Context()))
			var o st.Outcome
			select {
			case o = <-recovered:
			case <-time.After(5 * time.Second):
				t.Fatal("scheduler did not reconcile the reserved candidate")
			}
			require.Zero(t, calls.Load())
			require.NoError(t, db.First(&reserved, reserved.ID).Error)
			want := "interrupted"
			status := st.Failed
			if effect == "after-docker" {
				want = "restarted"
				status = st.Succeeded
			}
			require.Equal(t, want, reserved.Outcome)
			found := false
			for _, target := range o.Targets {
				if target.ID == "container" {
					found = true
					require.Equal(t, status, target.Status)
				}
			}
			require.True(t, found, "recovery appends the target missing from scheduler progress")
			var n int64
			require.NoError(t, db.Model(&activity.Activity{}).Count(&n).Error)
			require.EqualValues(t, 1, n)
			require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
			require.Zero(t, n, "recovered outcomes never notify")
		})
	}
}
func TestAutoHealUsesBoundedInspectionConcurrency(t *testing.T) {
	j, _ := autoHealFixture(t)
	j.listContainers = func(context.Context, *client.Client) ([]container.Summary, error) {
		return []container.Summary{{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}, {ID: "5"}}, nil
	}
	entered := make(chan struct{}, 5)
	release := make(chan struct{})
	var current, max atomic.Int32
	j.inspectContainer = func(_ context.Context, _ *client.Client, id string) (container.InspectResponse, error) {
		n := current.Add(1)
		for {
			m := max.Load()
			if n <= m || max.CompareAndSwap(m, n) {
				break
			}
		}
		entered <- struct{}{}
		<-release
		current.Add(-1)
		return healInspect(id), nil
	}
	done := make(chan error, 1)
	go func() { _, err := j.Run(t.Context()); done <- err }()
	for range 4 {
		<-entered
	}
	require.EqualValues(t, 4, max.Load())
	close(release)
	require.NoError(t, <-done)
	require.LessOrEqual(t, max.Load(), int32(4))
}
func TestAutoHealListAndSelfExclusion(t *testing.T) {
	opts := autoHealListOptionsInternal()
	require.False(t, opts.All)
	require.Equal(t, make(client.Filters).Add("health", string(container.Unhealthy)), opts.Filters)
	j := &AutoHealJob{}
	rows := j.filterCandidatesInternal([]container.Summary{{ID: "self123", Names: []string{"/hub"}}, {ID: "other", Names: []string{"/web"}}}, nil, "self")
	require.Len(t, rows, 1)
	require.Equal(t, "other", rows[0].ID)
}

func TestAutoHealOutOfContractSettingsFailClosedVisibly(t *testing.T) {
	j, db := autoHealFixture(t)
	require.NoError(t, j.settingsService.SetStringSetting(t.Context(), "autoHealMaxRestarts", "20"))
	var calls atomic.Int32
	j.restartContainer = func(context.Context, *client.Client, string) error { calls.Add(1); return nil }
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	_, err := j.Run(t.Context())
	require.NoError(t, err)
	require.Zero(t, calls.Load())
	var n int64
	require.NoError(t, db.Model(&autoheal.Restart{}).Count(&n).Error)
	require.Zero(t, n)
	require.Equal(t, 1, strings.Count(logs.String(), "auto-heal settings outside the typed contract"))
	require.Contains(t, logs.String(), "field=/maxRestarts")
}
