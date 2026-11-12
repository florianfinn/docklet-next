// Source: docklet-next/issues/440
// Revision: 45ee2635d68e71671b571cbd8970425ddd194b59
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original instant observation clock for tests outside the stability package.

// Package stabilitytest provides a test clock for observed updates.
package stabilitytest

import (
	"context"
	"sync"
	"time"
)

// Clock advances only when the observer waits or a test jumps it. Deadlines are
// reported in its time domain and capped by a parent deadline like a real context.
type Clock struct {
	mu    sync.Mutex
	now   time.Time
	start time.Time
}

func NewClock() *Clock {
	now := time.Now()
	return &Clock{now: now, start: now}
}
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Elapsed is the clock time spent since NewClock.
func (c *Clock) Elapsed() time.Duration { return c.Now().Sub(c.start) }

// Jump models time spent outside the observer, such as slow workload effects.
func (c *Clock) Jump(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type deadline struct {
	context.Context
	at time.Time
}

func (c deadline) Deadline() (time.Time, bool) { return c.at, true }
func (c *Clock) WithDeadline(parent context.Context, at time.Time) (context.Context, context.CancelFunc) {
	if outer, ok := parent.Deadline(); ok && outer.Before(at) {
		at = outer
	}
	child, cancel := context.WithCancel(parent)
	return deadline{Context: child, at: at}, cancel
}
func (c *Clock) Wait(ctx context.Context, until time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if until.After(c.now) {
		c.now = until
	}
	return nil
}
