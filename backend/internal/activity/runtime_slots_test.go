// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Migrated SQLite, actual settings/limiter and finite slot/lifetime barriers.

package activity

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/settings"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
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
	owned, cancel := context.WithTimeout(app, 20*time.Second)
	t.Cleanup(cancel)
	f.service = NewRuntimeService(svc, f.resolver, RuntimeLocalConfig{ApplicationContext: owned, Store: f.command().Store, Executor: execute})
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
	detail, err := f.store.svc.GetActivityDetail(t.Context(), "0", id, 100)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusQueued, detail.Activity.Status)
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
func slotsEvent(t *testing.T, events <-chan activitytypes.StreamEvent, id string, status activitytypes.Status) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		select {
		case event := <-events:
			if event.Activity != nil && event.Activity.ID == id {
				require.Equal(t, status, event.Activity.Status)
				return
			}
		case <-ctx.Done():
			t.Fatal("committed activity event was not published")
			return
		}
	}
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
	events, _, unsubscribe := f.store.svc.Subscribe("0")
	defer unsubscribe()
	request, disconnect := context.WithCancel(t.Context())
	r, err := f.service.Admit(request, c2)
	require.NoError(t, err)
	disconnect()
	require.Equal(t, activitytypes.StatusQueued, r.Activity.Status)
	slotsBlocked(t, f, c2, r.ActivityID, second)
	slotsEvent(t, events, r.ActivityID, activitytypes.StatusQueued)
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
	slotsEvent(t, events, r.ActivityID, activitytypes.StatusRunning)
	second.unblock()
	_, err = b2Wait(t, f, c2, r.ActivityID)
	require.NoError(t, err)
	slotsEvent(t, events, r.ActivityID, activitytypes.StatusSuccess)
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

func TestRuntimeSlotsDefaultUnlimitedAndNilSettings(t *testing.T) {
	for _, mode := range []string{"default-five", "zero", "negative", "nil-settings", "nil-limiter"} {
		t.Run(mode, func(t *testing.T) {
			var limit *int
			if mode == "zero" {
				limit = new(0)
			} else if mode == "negative" {
				limit = new(-2)
			}
			probes := make([]*slotsProbe, 6)
			for i := range probes {
				probes[i] = slotsNewProbe(t)
			}
			execute := b2ExecutorFunc(func(ctx context.Context, c RuntimeCommand, _ io.Writer) error {
				var i int
				_, err := fmt.Sscanf(c.Target.ID, "example-stack-%d", &i)
				if err != nil {
					return err
				}
				return probes[i].run(ctx)
			})
			var f *b1FixtureInternal
			if mode == "nil-settings" || mode == "nil-limiter" {
				f = b2Fixture(t, t.Context(), execute)
			} else {
				f = slotsFixture(t, t.Context(), limit, execute)
			}
			if mode == "nil-limiter" {
				f.store.svc.limiter = nil
			}
			slotsCleanup(t, f, probes...)
			ids := make([]string, 6)
			commands := make([]RuntimeCommand, 6)
			for i := range probes {
				commands[i] = slotsCommand(f, fmt.Sprintf("example-stack-%d", i))
				ids[i] = slotsAdmit(t, f, commands[i])
				if mode == "default-five" && i == 5 {
					slotsBlocked(t, f, commands[i], ids[i], probes[i])
					slotsHeld(t, f, 5)
				} else {
					b1Await(t, probes[i].entered)
				}
			}
			if mode == "zero" || mode == "negative" {
				slotsHeld(t, f, 6)
			}
			if mode == "default-five" {
				probes[0].unblock()
				_, err := b2Wait(t, f, commands[0], ids[0])
				require.NoError(t, err)
				b1Await(t, probes[5].entered)
			}
			for i, p := range probes {
				p.unblock()
				_, err := b2Wait(t, f, commands[i], ids[i])
				require.NoError(t, err)
			}
			if mode != "nil-limiter" {
				slotsHeld(t, f, 0)
			}
		})
	}
}

func TestRuntimeSlotsLiveSettingsKeepExistingHolders(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		initial, changed, holders int
	}{
		{"unlimited-to-finite", 0, 1, 2}, {"decrease", 2, 1, 2}, {"increase", 1, 2, 1}, {"finite-to-unlimited", 1, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probes := []*slotsProbe{slotsNewProbe(t), slotsNewProbe(t), slotsNewProbe(t)}
			f := slotsFixture(t, t.Context(), new(tc.initial), func(ctx context.Context, c RuntimeCommand, _ io.Writer) error {
				var i int
				_, err := fmt.Sscanf(c.Target.ID, "example-stack-%d", &i)
				if err != nil {
					return err
				}
				return probes[i].run(ctx)
			})
			slotsCleanup(t, f, probes...)
			ids := make([]string, 3)
			commands := make([]RuntimeCommand, 3)
			for i := 0; i < tc.holders; i++ {
				commands[i] = slotsCommand(f, fmt.Sprintf("example-stack-%d", i))
				ids[i] = slotsAdmit(t, f, commands[i])
				b1Await(t, probes[i].entered)
			}
			if tc.holders == 2 {
				require.NoError(t, f.store.svc.settings.SetIntSetting(t.Context(), maxConcurrentActivitiesSettingKey, tc.changed))
			}
			next := tc.holders
			commands[next] = slotsCommand(f, fmt.Sprintf("example-stack-%d", next))
			ids[next] = slotsAdmit(t, f, commands[next])
			slotsBlocked(t, f, commands[next], ids[next], probes[next])
			slotsHeld(t, f, tc.holders)
			if tc.holders == 1 {
				require.NoError(t, f.store.svc.settings.SetIntSetting(t.Context(), maxConcurrentActivitiesSettingKey, tc.changed))
				require.Equal(t, tc.changed, f.store.svc.settings.GetIntSetting(t.Context(), maxConcurrentActivitiesSettingKey, 99))
				// No release wakes this waiter: the unchanged two-second settings recheck must admit it.
				b1Await(t, probes[next].entered)
				slotsHeld(t, f, 2)
			} else {
				probes[0].unblock()
				_, err := b2Wait(t, f, commands[0], ids[0])
				require.NoError(t, err)
				slotsBlocked(t, f, commands[next], ids[next], probes[next])
				slotsHeld(t, f, 1)
				probes[1].unblock()
				_, err = b2Wait(t, f, commands[1], ids[1])
				require.NoError(t, err)
				b1Await(t, probes[next].entered)
				slotsHeld(t, f, 1)
			}
			for i := 0; i <= next; i++ {
				probes[i].unblock()
				_, err := b2Wait(t, f, commands[i], ids[i])
				require.NoError(t, err)
			}
			slotsHeld(t, f, 0)
		})
	}
}

func TestRuntimeSlotsQueuedCancellationAndLifetime(t *testing.T) {
	for _, mode := range []string{"before-registration", "queued-user", "app-stop", "earlier-deadline"} {
		t.Run(mode, func(t *testing.T) {
			app, stop := context.WithCancel(t.Context())
			defer stop()
			if mode == "earlier-deadline" {
				app, stop = context.WithTimeout(t.Context(), 750*time.Millisecond)
				defer stop()
			}
			probe := slotsNewProbe(t)
			f := slotsFixture(t, app, new(1), func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error { return probe.run(ctx) })
			slotsCleanup(t, f, probe)
			legacy := slotsLegacy(t, f, "0")
			if mode == "before-registration" {
				require.NoError(t, f.store.svc.db.Exec(`CREATE TRIGGER fixture_slots_precancel AFTER INSERT ON activity_messages WHEN NEW.code = 'run.started'
    BEGIN UPDATE runtime_receipts SET cancel_requested = 1, cancel_requested_at = admitted_at WHERE run_id = NEW.activity_id; END`).Error)
			}
			c := f.command()
			id := slotsAdmit(t, f, c)
			if mode != "before-registration" {
				slotsBlocked(t, f, c, id, probe)
			}
			if mode == "queued-user" {
				for range 2 {
					snapshot, err := f.service.Cancel(t.Context(), b2Ref(c, id), c.Actor)
					require.NoError(t, err)
					require.True(t, snapshot.CancelRequested)
				}
			} else if mode == "app-stop" {
				stop()
			}
			snapshot, err := b2Wait(t, f, c, id)
			require.NoError(t, err)
			want := activitytypes.StatusFailed
			if mode == "before-registration" || mode == "queued-user" {
				want = activitytypes.StatusCancelled
				require.Equal(t, "runtime.cancelled", snapshot.MessageCode)
			} else {
				require.Equal(t, "runtime.interrupted", snapshot.MessageCode)
			}
			require.Equal(t, want, snapshot.Status)
			require.Zero(t, probe.calls.Load())
			require.Nil(t, b3Receipt(t, f, id).ExecutorStartedAt)
			slotsHeld(t, f, 1)
			require.False(t, slotsRegistered(f, id))
			b2FinalCount(t, f, id, 1)
			slotsCompleteLegacy(t, f, legacy)
			slotsHeld(t, f, 0)
			late, err := f.service.Cancel(t.Context(), b2Ref(c, id), c.Actor)
			require.NoError(t, err)
			require.Equal(t, snapshot, late)
		})
	}
}

func TestRuntimeSlotsGrantRacingDurableCancelRetainedUntilCommit(t *testing.T) {
	first, second := slotsNewProbe(t), slotsNewProbe(t)
	f := slotsFixture(t, t.Context(), new(1), func(ctx context.Context, c RuntimeCommand, _ io.Writer) error {
		if c.Target.ID == "example-first" {
			return first.run(ctx)
		}
		return second.run(ctx)
	})
	slotsCleanup(t, f, first, second)
	c := slotsCommand(f, "example-first")
	c.ProposedRunID = uuid.NewString()
	// Cancellation becomes durable in the accepting transaction after its initial
	// intent read. Capacity is free; the legacy limiter is allowed to grant it.
	require.NoError(t, f.store.svc.db.Exec(`CREATE TRIGGER fixture_slots_race AFTER UPDATE OF executor_state ON runtime_receipts
 WHEN NEW.executor_state = 'accepted' AND NEW.run_id = '`+c.ProposedRunID+`'
 BEGIN UPDATE runtime_receipts SET cancel_requested = 1, cancel_requested_at = admitted_at WHERE run_id = NEW.run_id; END`).Error)
	b2Deferred(t, f, "NEW.code = 'runtime.cancelled'")
	id := slotsAdmit(t, f, c)
	owner := b2Owner(t, f, id)
	b1Await(t, owner.attemptDone)
	f.store.svc.runningMu.Lock()
	persistErr := owner.err
	result := *owner.result
	f.store.svc.runningMu.Unlock()
	require.ErrorContains(t, persistErr, "FOREIGN KEY constraint failed")
	require.Equal(t, "cancelled", result.state)
	require.Zero(t, first.calls.Load())
	slotsHeld(t, f, 1)
	require.True(t, slotsRegistered(f, id))
	require.Nil(t, b3Receipt(t, f, id).ExecutorStartedAt)
	other := slotsCommand(f, "example-second")
	otherID := slotsAdmit(t, f, other)
	slotsBlocked(t, f, other, otherID, second)
	require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER fixture_b2_fault").Error)
	snapshot, err := f.service.RetrySettlement(t.Context(), b2Ref(c, id))
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusCancelled, snapshot.Status)
	b1Await(t, second.entered)
	slotsHeld(t, f, 1)
	// Repeats must not release the second run's capacity.
	_, err = f.service.RetrySettlement(t.Context(), b2Ref(c, id))
	require.NoError(t, err)
	slotsHeld(t, f, 1)
	second.unblock()
	_, err = b2Wait(t, f, other, otherID)
	require.NoError(t, err)
	slotsHeld(t, f, 0)
	b2FinalCount(t, f, id, 1)
}

func TestRuntimeSlotsOutputJoinRetainsCapacity(t *testing.T) {
	first, second := slotsNewProbe(t), slotsNewProbe(t)
	returned, appendEntered, appendRelease := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblockAppend := func() { once.Do(func() { close(appendRelease) }) }
	defer unblockAppend()
	f := slotsFixture(t, t.Context(), new(1), func(ctx context.Context, c RuntimeCommand, w io.Writer) error {
		if c.Target.ID == "example-first" {
			_, err := io.WriteString(w, "first\nfinal partial")
			close(returned)
			return err
		}
		return second.run(ctx)
	})
	slotsCleanup(t, f, first, second)
	// Register after owner cleanup so an assertion failure unblocks output first.
	t.Cleanup(unblockAppend)
	var entered sync.Once
	f.service.outputAppender = &b2OutputAppender{ActivityService: f.store.svc, before: func(_ context.Context, _ string, _ []activitylib.AppendMessageRequest) error {
		entered.Do(func() { close(appendEntered) })
		<-appendRelease
		return nil
	}}
	c := slotsCommand(f, "example-first")
	id := slotsAdmit(t, f, c)
	b1Await(t, returned)
	b1Await(t, appendEntered)
	other := slotsCommand(f, "example-second")
	otherID := slotsAdmit(t, f, other)
	slotsBlocked(t, f, other, otherID, second)
	slotsHeld(t, f, 1)
	require.True(t, slotsRegistered(f, id))
	b2FinalCount(t, f, id, 0)
	unblockAppend()
	_, err := b2Wait(t, f, c, id)
	require.NoError(t, err)
	b1Await(t, second.entered)
	slotsHeld(t, f, 1)
	row := b3Activity(t, f, id)
	require.Len(t, row.Output, 2)
	require.Equal(t, "final partial", row.Output[1].Text)
	var codes []string
	require.NoError(t, f.store.svc.db.Raw("SELECT code FROM activity_messages WHERE activity_id = ? ORDER BY rowid", id).Scan(&codes).Error)
	require.Equal(t, []string{"run.started", "activity.message", "activity.message", "activity.success"}, codes)
	second.unblock()
	_, err = b2Wait(t, f, other, otherID)
	require.NoError(t, err)
	slotsHeld(t, f, 0)
}

func TestRuntimeSlotsTerminalFaultRetainsCapacityAndFrozenResult(t *testing.T) {
	for _, fault := range []string{"final-message", "activity-write", "commit"} {
		t.Run(fault, func(t *testing.T) {
			first, second := slotsNewProbe(t), slotsNewProbe(t)
			f := slotsFixture(t, t.Context(), new(1), func(ctx context.Context, c RuntimeCommand, _ io.Writer) error {
				if c.Target.ID == "example-first" {
					return first.run(ctx)
				}
				return second.run(ctx)
			})
			slotsCleanup(t, f, first, second)
			c := slotsCommand(f, "example-first")
			id := slotsAdmit(t, f, c)
			b1Await(t, first.entered)
			owner := b2Owner(t, f, id)
			before := runtimeBoundarySnapshot(t, f.store)
			switch fault {
			case "final-message":
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_slots_fault BEFORE INSERT ON activity_messages WHEN NEW.code = 'activity.success' BEGIN SELECT RAISE(ABORT, 'fixture final message'); END").Error)
			case "activity-write":
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_slots_fault BEFORE UPDATE OF status ON activities WHEN NEW.status = 'success' BEGIN SELECT RAISE(ABORT, 'fixture terminal activity'); END").Error)
			case "commit":
				b2Deferred(t, f, "NEW.code = 'activity.success'")
			}
			first.unblock()
			b1Await(t, owner.attemptDone)
			f.store.svc.runningMu.Lock()
			persistErr := owner.err
			frozen := *owner.result
			f.store.svc.runningMu.Unlock()
			require.Error(t, persistErr)
			if fault == "commit" {
				require.ErrorContains(t, persistErr, "FOREIGN KEY constraint failed")
			}
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			_, err := f.service.SettleLocalStartup(t.Context(), c.Store)
			require.ErrorIs(t, err, ErrRuntimeStartupOwnersLive)
			conflicting := c
			conflicting.Key = uuid.NewString()
			conflicting.Operation = RuntimeDownProject
			_, err = f.service.Admit(t.Context(), conflicting)
			var busy *GateConflict
			require.ErrorAs(t, err, &busy)
			other := slotsCommand(f, "example-second")
			otherID := slotsAdmit(t, f, other)
			slotsBlocked(t, f, other, otherID, second)
			slotsHeld(t, f, 1)
			require.True(t, slotsRegistered(f, id))
			trigger := "fixture_slots_fault"
			if fault == "commit" {
				trigger = "fixture_b2_fault"
			}
			require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER "+trigger).Error)
			var wg sync.WaitGroup
			errors := make(chan error, 8)
			for range 8 {
				wg.Go(func() {
					_, e := f.service.Cancel(t.Context(), b2Ref(c, id), c.Actor)
					if e == nil {
						_, e = f.service.RetrySettlement(t.Context(), b2Ref(c, id))
					}
					errors <- e
				})
			}
			wg.Wait()
			close(errors)
			for e := range errors {
				require.NoError(t, e)
			}
			b1Await(t, second.entered)
			slotsHeld(t, f, 1)
			require.False(t, slotsRegistered(f, id))
			require.True(t, slotsRegistered(f, otherID))
			require.Equal(t, new(frozen.settledAt), b3Receipt(t, f, id).ExecutorSettledAt)
			require.Equal(t, "success", *b3Receipt(t, f, id).ExecutorState)
			b2FinalCount(t, f, id, 1)
			require.EqualValues(t, 1, first.calls.Load())
			require.EqualValues(t, 1, second.calls.Load())
			second.unblock()
			_, err = b2Wait(t, f, other, otherID)
			require.NoError(t, err)
			slotsHeld(t, f, 0)
		})
	}
}

func TestRuntimeSlotsInterruptedChildHoldsLegacyWaiter(t *testing.T) {
	for _, mode := range []string{"user", "application", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			app, stop := context.WithCancel(t.Context())
			defer stop()
			if mode == "deadline" {
				app, stop = context.WithTimeout(t.Context(), 750*time.Millisecond)
				defer stop()
			}
			probe := slotsNewProbe(t)
			f := slotsFixture(t, app, new(1), func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error {
				joined := make(chan struct{})
				go func() { defer close(joined); _ = probe.run(ctx) }()
				<-joined
				return ctx.Err()
			})
			slotsCleanup(t, f, probe)
			c := f.command()
			id := slotsAdmit(t, f, c)
			b1Await(t, probe.entered)
			owner := b2Owner(t, f, id)
			legacy := slotsLegacy(t, f, "0")
			wait, cancel := context.WithTimeout(t.Context(), 8*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- f.store.svc.AwaitActivitySlot(wait, legacy, "0") }()
			t.Cleanup(cancel)
			if mode == "user" {
				_, err := f.service.Cancel(t.Context(), b2Ref(c, id), c.Actor)
				require.NoError(t, err)
			} else if mode == "application" {
				stop()
			}
			b1Await(t, owner.ctx.Done())
			premature := false
			select {
			case <-done:
				premature = true
			case <-time.After(120 * time.Millisecond):
			}
			require.False(t, premature, "legacy contender acquired while a cancelled child was still live")
			slotsHeld(t, f, 1)
			require.True(t, slotsRegistered(f, id))
			b2FinalCount(t, f, id, 0)
			require.Equal(t, activitytypes.StatusQueued, b3Activity(t, f, legacy).Status)
			probe.unblock()
			_, err := b2Wait(t, f, c, id)
			require.NoError(t, err)
			select {
			case err = <-done:
				require.NoError(t, err)
			case <-wait.Done():
				t.Fatal("legacy waiter did not acquire after actual child join")
			}
			slotsHeld(t, f, 1)
			slotsCompleteLegacy(t, f, legacy)
			slotsHeld(t, f, 0)
		})
	}
}

func TestRuntimeSlotsAdmissionAndExecutingFaultsNeverEffect(t *testing.T) {
	for _, mode := range []string{"queued-status", "queued-status-ignored", "accepted-commit", "executing-status", "executing-status-ignored", "executing-commit"} {
		t.Run(mode, func(t *testing.T) {
			probe := slotsNewProbe(t)
			f := slotsFixture(t, t.Context(), new(1), func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error {
				if mode == "executing-status-ignored" {
					probe.calls.Add(1)
					return nil
				}
				return probe.run(ctx)
			})
			slotsCleanup(t, f, probe)
			c := f.command()
			before := runtimeBoundarySnapshot(t, f.store)
			if mode == "queued-status" || mode == "queued-status-ignored" {
				statement := "SELECT RAISE(ABORT, 'fixture queue status');"
				if mode == "queued-status-ignored" {
					statement = "SELECT RAISE(IGNORE);"
				}
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_slots_fault BEFORE UPDATE OF status ON activities WHEN NEW.status = 'queued' BEGIN "+statement+" END").Error)
				result, err := f.service.Admit(t.Context(), c)
				require.Error(t, err)
				b1Zero(t, result)
				require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
				require.Empty(t, f.service.owners)
				slotsHeld(t, f, 0)
				require.Zero(t, probe.calls.Load())
				return
			}
			// Also fail terminal persistence to inspect the rolled-back phase and real
			// grant before storage-only retry. No generic failure release is permitted.
			require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_slots_terminal BEFORE INSERT ON activity_messages WHEN NEW.code = 'runtime.failed' BEGIN SELECT RAISE(ABORT, 'fixture retain phase failure'); END").Error)
			if mode == "executing-status" || mode == "executing-status-ignored" {
				statement := "SELECT RAISE(ABORT, 'fixture running status');"
				if mode == "executing-status-ignored" {
					statement = "SELECT RAISE(IGNORE);"
				}
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_slots_fault BEFORE UPDATE OF status ON activities WHEN NEW.status = 'running' BEGIN "+statement+" END").Error)
			} else {
				require.NoError(t, f.store.svc.db.Exec("CREATE TABLE fixture_slots_parent(id INTEGER PRIMARY KEY)").Error)
				require.NoError(t, f.store.svc.db.Exec("CREATE TABLE fixture_slots_deferred(id INTEGER REFERENCES fixture_slots_parent(id) DEFERRABLE INITIALLY DEFERRED)").Error)
				state := "executing"
				if mode == "accepted-commit" {
					state = "accepted"
				}
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_slots_fault AFTER UPDATE OF executor_state ON runtime_receipts WHEN NEW.executor_state = '"+state+"' BEGIN INSERT INTO fixture_slots_deferred VALUES(1); END").Error)
			}
			id := slotsAdmit(t, f, c)
			owner := b2Owner(t, f, id)
			b1Await(t, owner.attemptDone)
			require.Zero(t, probe.calls.Load())
			row := b3Receipt(t, f, id)
			require.Nil(t, row.ExecutorStartedAt)
			require.Nil(t, row.SettledAt)
			require.Equal(t, activitytypes.StatusQueued, b3Activity(t, f, id).Status)
			held := 1
			state := "accepted"
			if mode == "accepted-commit" {
				held = 0
				state = "pending"
			}
			require.Equal(t, state, *row.ExecutorState)
			slotsHeld(t, f, held)
			require.Equal(t, held == 1, slotsRegistered(f, id))
			f.store.svc.runningMu.Lock()
			persistErr, resultState := owner.err, owner.result.state
			f.store.svc.runningMu.Unlock()
			require.Error(t, persistErr)
			require.Equal(t, "failed", resultState)
			require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER fixture_slots_fault").Error)
			require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER fixture_slots_terminal").Error)
			snapshot, err := f.service.RetrySettlement(t.Context(), b2Ref(c, id))
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusFailed, snapshot.Status)
			slotsHeld(t, f, 0)
			b2FinalCount(t, f, id, 1)
			require.Zero(t, probe.calls.Load())
		})
	}
}
