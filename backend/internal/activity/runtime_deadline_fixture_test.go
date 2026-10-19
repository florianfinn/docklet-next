// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Test-controlled application deadline expiry after durable phase barriers.

package activity

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The application clock advances to its deadline only at expire, after fixture
// setup and the tested phase barrier. Done, Err and Cause then expose deadline
// expiry through the real context propagation, limiter and settlement paths.
// Value hides the parent's cancellation state as well as its real-time clock.
type runtimeDeadlineFixture struct {
	context.Context
	deadline time.Time
	done     chan struct{}
	mu       sync.Mutex
	err      error
}

func newRuntimeDeadlineFixture(t *testing.T) *runtimeDeadlineFixture {
	t.Helper()
	c := &runtimeDeadlineFixture{Context: context.WithoutCancel(t.Context()),
		deadline: time.Now().Add(750 * time.Millisecond), done: make(chan struct{})}
	t.Cleanup(func() { c.finish(context.Canceled) })
	return c
}

func (c *runtimeDeadlineFixture) Deadline() (time.Time, bool) { return c.deadline, true }
func (c *runtimeDeadlineFixture) Done() <-chan struct{}       { return c.done }
func (c *runtimeDeadlineFixture) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}
func (c *runtimeDeadlineFixture) finish(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		c.err = err
		close(c.done)
	}
}
func (c *runtimeDeadlineFixture) assertInherited(t *testing.T, owner context.Context) {
	t.Helper()
	deadline, ok := owner.Deadline()
	require.True(t, ok)
	require.Equal(t, c.deadline, deadline, "owner must inherit the earlier application deadline")
	require.True(t, deadline.Before(time.Now().Add(runtimeWorkflowLimit)))
	require.NoError(t, c.Err())
	require.NoError(t, owner.Err(), "application clock has not expired at the phase barrier")
}
func (c *runtimeDeadlineFixture) expire(t *testing.T, owner context.Context) {
	t.Helper()
	c.assertInherited(t, owner)
	c.finish(context.DeadlineExceeded)
	b1Await(t, owner.Done())
	require.ErrorIs(t, c.Err(), context.DeadlineExceeded)
	require.ErrorIs(t, context.Cause(c), context.DeadlineExceeded)
	require.ErrorIs(t, owner.Err(), context.DeadlineExceeded)
	require.ErrorIs(t, context.Cause(owner), context.DeadlineExceeded)
}
