// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original deterministic observation clock and engine doubles.

package stability

import (
	"context"
	"sync"
	"testing"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
)

type deadlineContext struct {
	context.Context
	deadline time.Time
}

func (c deadlineContext) Deadline() (time.Time, bool) { return c.deadline, true }
func (c deadlineContext) Err() error {
	if context.Cause(c.Context) == context.DeadlineExceeded {
		return context.DeadlineExceeded
	}
	return c.Context.Err()
}

type fakeDeadline struct {
	at     time.Time
	cancel context.CancelCauseFunc
}

type fakeClock struct {
	mu        sync.Mutex
	now       time.Time
	deadlines map[*fakeDeadline]bool
	onWait    func(time.Time)
	waits     []time.Time
}

func newClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), deadlines: map[*fakeDeadline]bool{}}
}
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}
func (c *fakeClock) advance(at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if at.After(c.now) {
		c.now = at
	}
	for deadline := range c.deadlines {
		if !c.now.Before(deadline.at) {
			deadline.cancel(context.DeadlineExceeded)
		}
	}
}
func (c *fakeClock) WithDeadline(parent context.Context, at time.Time) (context.Context, context.CancelFunc) {
	if outer, ok := parent.Deadline(); ok && outer.Before(at) {
		at = outer
	}
	child, cancel := context.WithCancelCause(parent)
	deadline := &fakeDeadline{at: at, cancel: cancel}
	c.mu.Lock()
	c.deadlines[deadline] = true
	if !c.now.Before(at) {
		cancel(context.DeadlineExceeded)
	}
	c.mu.Unlock()
	return deadlineContext{Context: child, deadline: at}, func() {
		cancel(context.Canceled)
		c.mu.Lock()
		delete(c.deadlines, deadline)
		c.mu.Unlock()
	}
}
func (c *fakeClock) Wait(ctx context.Context, at time.Time) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	c.waits = append(c.waits, at)
	if c.onWait != nil {
		c.onWait(at)
	}
	c.advance(at)
	return ctx.Err()
}

type fakeEngine struct {
	list    func(context.Context, string, string) ([]Instance, error)
	inspect func(context.Context, string) (Container, error)
}

func (e fakeEngine) List(ctx context.Context, project, service string) ([]Instance, error) {
	return e.list(ctx, project, service)
}
func (e fakeEngine) Inspect(ctx context.Context, id string) (Container, error) {
	return e.inspect(ctx, id)
}
func running(id string, started time.Time) Container {
	return Container{ID: id, State: "running", StartedAt: started}
}
func stackSet(t *testing.T, want int, job bool) RequiredSet {
	t.Helper()
	services := compose.Services{"app": {Name: "app", Scale: &want}}
	if job {
		services["consumer"] = compose.ServiceConfig{DependsOn: compose.DependsOnConfig{"app": {Condition: compose.ServiceConditionCompletedSuccessfully}}}
	}
	set, err := NewStackSet(&compose.Project{Name: "example", Services: services}, []string{"app"})
	if err != nil {
		t.Fatal(err)
	}
	return set
}
func expectFail(t *testing.T, verdict Verdict) {
	t.Helper()
	if verdict.Pass || verdict.Cancelled || verdict.Code != CodeUnstable {
		t.Fatalf("want unstable verdict, got %+v", verdict)
	}
}
func expectPass(t *testing.T, verdict Verdict, clock *fakeClock, t0 time.Time) {
	t.Helper()
	if !verdict.Pass || verdict.Cancelled || verdict.Code != "" {
		t.Fatalf("want pass, got %+v", verdict)
	}
	if got := clock.Now().Sub(t0); got != Window {
		t.Fatalf("observation lasted %v, want %v", got, Window)
	}
}
func observeOne(clock *fakeClock, change func(time.Duration, *Container)) Verdict {
	t0 := clock.Now()
	engine := fakeEngine{inspect: func(_ context.Context, id string) (Container, error) {
		result := running(id, t0)
		change(clock.Now().Sub(t0), &result)
		return result, nil
	}}
	return New(clock, engine).Observe(context.Background(), NewContainerSet("a"), t0)
}
