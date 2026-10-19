// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Migrated SQLite, actual settings/limiter and finite slot/lifetime barriers.

package activity

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/settings"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type slotsProbe struct {
	entered, release chan struct{}
	once             sync.Once
	calls            atomic.Int64
}

func slotsNewProbe(t *testing.T) *slotsProbe {
	t.Helper()
	p := &slotsProbe{entered: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(p.unblock)
	return p
}
func (p *slotsProbe) unblock() { p.once.Do(func() { close(p.release) }) }
func (p *slotsProbe) run(ctx context.Context) error {
	p.calls.Add(1)
	close(p.entered)
	select {
	case <-p.release:
	case <-ctx.Done():
		<-p.release
	}
	return ctx.Err()
}
func slotsFixture(t *testing.T, app context.Context, limit *int, execute b2ExecutorFunc) *b1FixtureInternal {
	t.Helper()
	f := b1Fixture(t)
	config, err := settings.NewSettingsService(t.Context(), f.store.svc.db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, config.Stop(context.WithoutCancel(t.Context()))) })
	if limit != nil {
		require.NoError(t, config.SetIntSetting(t.Context(), maxConcurrentActivitiesSettingKey, *limit))
	}
	svc := NewActivityService(f.store.svc.db, config)
	svc.clock = f.store.svc.clock
	f.store.svc = svc
	f.service = NewRuntimeService(svc, f.resolver, RuntimeLocalConfig{ApplicationContext: app, Store: f.command().Store, Executor: execute})
	return f
}
func slotsCleanup(t *testing.T, f *b1FixtureInternal, probes ...*slotsProbe) {
	t.Helper()
	t.Cleanup(func() {
		for _, p := range probes {
			p.unblock()
		}
		f.store.svc.runningMu.Lock()
		owners := make([]*runtimeOwnerInternal, 0, len(f.service.owners))
		for _, owner := range f.service.owners {
			owner.cancel(context.Canceled)
			owners = append(owners, owner)
		}
		f.store.svc.runningMu.Unlock()
		for _, owner := range owners {
			b1Await(t, owner.attemptDone)
		}
	})
}
func slotsCommand(f *b1FixtureInternal, target string) RuntimeCommand {
	c := f.command()
	c.Target.ID = target
	return c
}
func slotsAdmit(t *testing.T, f *b1FixtureInternal, c RuntimeCommand) string {
	t.Helper()
	r, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusQueued, r.Activity.Status)
	require.Equal(t, c.Actor.ID, r.Activity.StartedBy.UserID)
	return r.ActivityID
}
func slotsHeld(t *testing.T, f *b1FixtureInternal, want int) {
	t.Helper()
	l := f.store.svc.limiter
	l.mu.Lock()
	got := l.stateForLockedInternal("0").held
	l.mu.Unlock()
	require.Equal(t, want, got, "actual shared limiter capacity")
}
func slotsRegistered(f *b1FixtureInternal, id string) bool {
	f.store.svc.slotMu.Lock()
	defer f.store.svc.slotMu.Unlock()
	_, ok := f.store.svc.slotReleases[id]
	return ok
}
func slotsAccepted(t *testing.T, f *b1FixtureInternal, id string) {
	t.Helper()
	require.Eventually(t, func() bool {
		var r RuntimeReceipt
		return f.store.svc.db.First(&r, "run_id = ?", id).Error == nil && *r.ExecutorState != "pending"
	}, 5*time.Second, 5*time.Millisecond)
}
func slotsBlocked(t *testing.T, f *b1FixtureInternal, c RuntimeCommand, id string, probe *slotsProbe) {
	t.Helper()
	slotsAccepted(t, f, id)
	premature := false
	select {
	case <-probe.entered:
		premature = true
	case <-time.After(120 * time.Millisecond):
	}
	require.False(t, premature, "contender entered before the slot release barrier")
	row := b3Receipt(t, f, id)
	require.Equal(t, "accepted", *row.ExecutorState)
	require.Nil(t, row.ExecutorStartedAt)
	require.Nil(t, row.SettledAt)
	require.Equal(t, activitytypes.StatusQueued, b3Activity(t, f, id).Status)
	require.Equal(t, activitytypes.StatusQueued, b3Projection(t, f, id).Status)
	require.False(t, slotsRegistered(f, id))
	var bindings, starts int64
	require.NoError(t, f.store.svc.db.Model(&RuntimeKeyBinding{}).Where("run_id = ?", id).Count(&bindings).Error)
	require.NoError(t, f.store.svc.db.Model(&ActivityMessage{}).Where("activity_id = ? AND code = 'run.started'", id).Count(&starts).Error)
	require.Positive(t, bindings)
	require.EqualValues(t, 1, starts)
	retry, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	require.Equal(t, id, retry.ActivityID)
	require.Equal(t, activitytypes.StatusQueued, retry.Activity.Status)
}
func slotsLegacy(t *testing.T, f *b1FixtureInternal, env string) string {
	t.Helper()
	r, err := f.store.svc.StartActivity(t.Context(), StartActivityRequest{EnvironmentID: env, Type: activitytypes.TypeImagePull, Queue: true})
	require.NoError(t, err)
	return r.ID
}
func slotsCompleteLegacy(t *testing.T, f *b1FixtureInternal, id string) {
	t.Helper()
	_, err := f.store.svc.CompleteActivity(t.Context(), id, activitytypes.StatusSuccess, "Complete", nil)
	require.NoError(t, err)
}

func TestRuntimeSlotsDurableQueueJoinAndSharedLegacy(t *testing.T) {
	first, second := slotsNewProbe(t), slotsNewProbe(t)
	f := slotsFixture(t, t.Context(), new(1), func(ctx context.Context, c RuntimeCommand, _ io.Writer) error {
		if c.Target.ID == "example-first" {
			return first.run(ctx)
		}
		return second.run(ctx)
	})
	slotsCleanup(t, f, first, second)
	c1, c2 := slotsCommand(f, "example-first"), slotsCommand(f, "example-second")
	id1 := slotsAdmit(t, f, c1)
	b1Await(t, first.entered)
	slotsHeld(t, f, 1)
	request, disconnect := context.WithCancel(t.Context())
	r, err := f.service.Admit(request, c2)
	require.NoError(t, err)
	disconnect()
	require.Equal(t, activitytypes.StatusQueued, r.Activity.Status)
	slotsBlocked(t, f, c2, r.ActivityID, second)
	joined := c2
	joined.Key = uuid.NewString()
	joined.Actor.ID = "example-joiner"
	join, err := f.service.Admit(t.Context(), joined)
	require.NoError(t, err)
	require.Equal(t, r.ActivityID, join.ActivityID)
	require.Equal(t, c2.Actor.ID, join.Activity.StartedBy.UserID)
	child := c2
	child.Key = uuid.NewString()
	child.Operation = RuntimeStartContainer
	child.Target = RuntimeTarget{Kind: "container", ID: "example-child", ParentProjectID: new(c2.Target.ID)}
	_, err = f.service.Admit(t.Context(), child)
	var busy *GateConflict
	require.ErrorAs(t, err, &busy)
	other := slotsLegacy(t, f, "example-other-environment")
	require.Equal(t, activitytypes.StatusRunning, b3Activity(t, f, other).Status)
	slotsCompleteLegacy(t, f, other)
	legacy := slotsLegacy(t, f, "0")
	require.Equal(t, activitytypes.StatusQueued, b3Activity(t, f, legacy).Status)
	first.unblock()
	_, err = b2Wait(t, f, c1, id1)
	require.NoError(t, err)
	b1Await(t, second.entered)
	slotsHeld(t, f, 1)
	require.True(t, slotsRegistered(f, r.ActivityID))
	require.Equal(t, activitytypes.StatusRunning, b3Projection(t, f, r.ActivityID).Status)
	second.unblock()
	_, err = b2Wait(t, f, c2, r.ActivityID)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, f.store.svc.AwaitActivitySlot(ctx, legacy, "0"))
	slotsHeld(t, f, 1)
	slotsCompleteLegacy(t, f, legacy)
	slotsHeld(t, f, 0)
	require.EqualValues(t, 1, first.calls.Load())
	require.EqualValues(t, 1, second.calls.Load())
}

func TestRuntimeSlotsLegacyFirstAndOtherFiveRemainUnqueued(t *testing.T) {
	for _, op := range []RuntimeOperation{RuntimeDownProject, RuntimeRestartProject, RuntimeStartContainer, RuntimeStopContainer, RuntimeRestartContainer, RuntimeRedeployProject} {
		t.Run(string(op), func(t *testing.T) {
			probe := slotsNewProbe(t)
			f := slotsFixture(t, t.Context(), new(1), func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error { return probe.run(ctx) })
			slotsCleanup(t, f, probe)
			legacy := slotsLegacy(t, f, "0")
			c := f.command()
			c.Operation = op
			if op == RuntimeStartContainer || op == RuntimeStopContainer || op == RuntimeRestartContainer {
				c.Target = RuntimeTarget{Kind: "container", ID: "example-container"}
			}
			r, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			if op == RuntimeRedeployProject {
				slotsBlocked(t, f, c, r.ActivityID, probe)
				slotsCompleteLegacy(t, f, legacy)
			} else {
				require.Equal(t, activitytypes.StatusRunning, r.Activity.Status)
			}
			b1Await(t, probe.entered)
			slotsHeld(t, f, 1)
			if op != RuntimeRedeployProject {
				require.False(t, slotsRegistered(f, r.ActivityID))
				slotsCompleteLegacy(t, f, legacy)
			}
			probe.unblock()
			_, err = b2Wait(t, f, c, r.ActivityID)
			require.NoError(t, err)
			slotsHeld(t, f, 0)
		})
	}
}
