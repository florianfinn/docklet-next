// Source: docklet-next/issues/440
// Revision: 45ee2635d68e71671b571cbd8970425ddd194b59
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original coverage of the pre-attempt baseline for a restored original.

package stability

import (
	"context"
	"testing"
	"time"
)

func observeSince(clock *fakeClock, count int, started time.Time, change func(time.Duration, *Container)) Verdict {
	t0 := clock.Now()
	engine := fakeEngine{inspect: func(_ context.Context, id string) (Container, error) {
		result := Container{ID: id, State: "running", RestartCount: count, StartedAt: started}
		change(clock.Now().Sub(t0), &result)
		return result, nil
	}}
	return New(clock, engine).Observe(context.Background(), NewContainerSetSince("a", count, started), t0)
}

// An original that compensation never stopped keeps its earlier policy restarts.
func TestBaselineUnchangedOriginalPasses(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	started := t0.Add(-time.Hour)
	expectPass(t, observeSince(clock, 3, started, func(time.Duration, *Container) {}), clock, t0)
}

// A fresh API start resets the counter; it passes against a non-zero baseline.
func TestBaselineFreshStartPasses(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	expectPass(t, observeSince(clock, 3, t0.Add(-time.Hour), func(_ time.Duration, c *Container) {
		c.RestartCount, c.StartedAt = 0, t0
	}), clock, t0)
}

// A policy restart before the first sample increments the counter and moves the
// start time; neither the baseline nor the fresh-start rule accepts it.
func TestBaselineRestartBeforeFirstSampleFails(t *testing.T) {
	for _, tc := range []struct {
		name     string
		baseline int
		count    int
	}{{"from baseline", 3, 4}, {"after fresh start", 3, 1}, {"from zero", 0, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			clock := newClock()
			t0 := clock.Now()
			expectFail(t, observeSince(clock, tc.baseline, t0.Add(-time.Hour), func(_ time.Duration, c *Container) {
				c.RestartCount, c.StartedAt = tc.count, t0
			}))
		})
	}
}

// The baseline only covers the first sample; a restart during W still fails.
func TestBaselineRestartDuringWindowFails(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	expectFail(t, observeSince(clock, 3, t0.Add(-time.Hour), func(elapsed time.Duration, c *Container) {
		if elapsed >= 10*time.Second {
			c.RestartCount, c.StartedAt = 4, t0.Add(elapsed)
		}
	}))
}
