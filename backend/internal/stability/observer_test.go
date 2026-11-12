// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original behavioural coverage of observation break-map rows.

package stability

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestO1OrdinaryExitZero(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	verdict := observeOne(clock, func(elapsed time.Duration, c *Container) {
		if elapsed >= 10*time.Second {
			c.State = "exited"
			c.ExitCode = 0
		}
	})
	expectFail(t, verdict)
	if clock.Now().Sub(t0) != 10*time.Second {
		t.Fatal("exit did not fail early")
	}
}

func TestO2MissingReplica(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	engine := fakeEngine{
		list: func(_ context.Context, project, service string) ([]Instance, error) {
			if project != "example" || service != "app" {
				t.Fatal("wrong compose label selection")
			}
			return []Instance{{ID: "a"}, {ID: "b"}}, nil
		},
		inspect: func(_ context.Context, id string) (Container, error) { return running(id, t0), nil },
	}
	expectFail(t, New(clock, engine).Observe(context.Background(), stackSet(t, 3, false), t0))
	if !clock.Now().Equal(t0.Add(Window)) {
		t.Fatal("missing replica must be evaluated at end of window")
	}
}

func TestO3aRestartDuringWindow(t *testing.T) {
	clock := newClock()
	verdict := observeOne(clock, func(elapsed time.Duration, c *Container) {
		if elapsed >= 10*time.Second {
			c.RestartCount = 1
		}
	})
	expectFail(t, verdict)
}

func TestO3bIdentitySwap(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	engine := fakeEngine{
		list: func(context.Context, string, string) ([]Instance, error) {
			id := "a"
			if clock.Now().Sub(t0) >= 10*time.Second {
				id = "b"
			}
			return []Instance{{ID: id}}, nil
		},
		inspect: func(_ context.Context, id string) (Container, error) { return running(id, t0), nil },
	}
	expectFail(t, New(clock, engine).Observe(context.Background(), stackSet(t, 1, false), t0))
	if clock.Now().Sub(t0) != 10*time.Second {
		t.Fatal("identity swap did not fail early")
	}
}

func TestO3cRestartBeforeFirstSample(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	expectFail(t, observeOne(clock, func(_ time.Duration, c *Container) { c.RestartCount = 1 }))
	if !clock.Now().Equal(t0) {
		t.Fatal("first-sample restart did not fail immediately")
	}
}

func TestO3dRestoredCounterReset(t *testing.T) {
	for _, phase := range []string{"stable", "before-first-sample", "during-window"} {
		t.Run(phase, func(t *testing.T) {
			clock := newClock()
			t0 := clock.Now()
			// Docker API Start resets a restored original's old counter of four to zero.
			old := running("a", t0.Add(-time.Hour))
			old.RestartCount = 4
			old.RestartCount = 0
			old.StartedAt = t0
			engine := fakeEngine{inspect: func(context.Context, string) (Container, error) {
				c := old
				elapsed := clock.Now().Sub(t0)
				if phase == "before-first-sample" || (phase == "during-window" && elapsed >= 10*time.Second) {
					c.RestartCount++
					c.StartedAt = t0.Add(time.Second)
				}
				return c, nil
			}}
			verdict := New(clock, engine).Observe(context.Background(), NewContainerSet(old.ID), t0)
			if phase == "stable" {
				expectPass(t, verdict, clock, t0)
			} else {
				expectFail(t, verdict)
			}
		})
	}
}

func TestStartedAtChangedWithoutCounter(t *testing.T) {
	clock := newClock()
	expectFail(t, observeOne(clock, func(elapsed time.Duration, c *Container) {
		if elapsed >= 10*time.Second {
			c.StartedAt = c.StartedAt.Add(time.Second)
		}
	}))
}

func TestO4HealthAtDeadline(t *testing.T) {
	for _, health := range []string{"starting", "healthy"} {
		t.Run(health, func(t *testing.T) {
			clock := newClock()
			t0 := clock.Now()
			verdict := observeOne(clock, func(_ time.Duration, c *Container) { c.Health = health })
			if health == "starting" {
				expectFail(t, verdict)
			} else {
				expectPass(t, verdict, clock, t0)
			}
			if !clock.Now().Equal(t0.Add(Window)) {
				t.Fatal("health decision shortened full window")
			}
		})
	}
}

func TestO4bHealthTransitions(t *testing.T) {
	t.Run("unhealthy-early", func(t *testing.T) {
		clock := newClock()
		t0 := clock.Now()
		verdict := observeOne(clock, func(elapsed time.Duration, c *Container) {
			c.Health = "healthy"
			if elapsed >= 10*time.Second && elapsed < 20*time.Second {
				c.Health = "unhealthy"
			}
		})
		expectFail(t, verdict)
		if clock.Now().Sub(t0) != 10*time.Second {
			t.Fatal("unhealthy did not fail early")
		}
	})
	t.Run("healthy-at-90-seconds", func(t *testing.T) {
		clock := newClock()
		t0 := clock.Now()
		verdict := observeOne(clock, func(elapsed time.Duration, c *Container) {
			c.Health = "starting"
			if elapsed >= 90*time.Second {
				c.Health = "healthy"
			}
		})
		expectPass(t, verdict, clock, t0)
	})
}

func TestO5CompletionJobs(t *testing.T) {
	for _, scenario := range []string{"exit-zero", "exit-one", "one-slot-still-running", "one-slot-missing"} {
		t.Run(scenario, func(t *testing.T) {
			clock := newClock()
			t0 := clock.Now()
			engine := fakeEngine{
				list: func(context.Context, string, string) ([]Instance, error) {
					if scenario == "one-slot-missing" {
						return []Instance{{ID: "a"}}, nil
					}
					return []Instance{{ID: "a"}, {ID: "b"}}, nil
				},
				inspect: func(_ context.Context, id string) (Container, error) {
					c := running(id, t0)
					if clock.Now().Sub(t0) >= 10*time.Second && !(scenario == "one-slot-still-running" && id == "b") {
						c.State = "exited"
						if scenario == "exit-one" {
							c.ExitCode = 1
						}
					}
					return c, nil
				},
			}
			verdict := New(clock, engine).Observe(context.Background(), stackSet(t, 2, true), t0)
			if scenario == "exit-zero" {
				expectPass(t, verdict, clock, t0)
			} else {
				expectFail(t, verdict)
			}
			if scenario == "exit-one" && clock.Now().Sub(t0) != 10*time.Second {
				t.Fatal("failed job did not fail early")
			}
		})
	}
}

func TestCompletedJobIsDone(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	engine := fakeEngine{
		list: func(context.Context, string, string) ([]Instance, error) {
			if clock.Now().After(t0) {
				return nil, nil
			}
			return []Instance{{ID: "a"}}, nil
		},
		inspect: func(context.Context, string) (Container, error) {
			return Container{ID: "a", State: "exited", StartedAt: t0}, nil
		},
	}
	expectPass(t, New(clock, engine).Observe(context.Background(), stackSet(t, 1, true), t0), clock, t0)
}

func TestO11SlowDeploy(t *testing.T) {
	clock := newClock()
	start := clock.Now()
	engine := fakeEngine{inspect: func(_ context.Context, id string) (Container, error) {
		return running(id, start.Add(3*time.Minute)), nil
	}}
	observer := New(clock, engine)
	// Joined deploy work consumes three minutes before the caller captures t0.
	clock.advance(start.Add(3 * time.Minute))
	t0 := clock.Now()
	// A delayed invocation must retain the caller's post-deploy boundary.
	clock.advance(t0.Add(time.Second))
	verdict := observer.Observe(context.Background(), NewContainerSet("a"), t0)
	expectPass(t, verdict, clock, t0)
	if clock.Now().Sub(start) != 5*time.Minute {
		t.Fatal("deploy consumed observation window")
	}
}

func TestO12CallerCancel(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clock.onWait = func(at time.Time) {
		if !at.Before(t0.Add(10 * time.Second)) {
			cancel()
		}
	}
	engine := fakeEngine{inspect: func(_ context.Context, id string) (Container, error) { return running(id, t0), nil }}
	verdict := New(clock, engine).Observe(ctx, NewContainerSet("a"), t0)
	if verdict.Pass || !verdict.Cancelled || verdict.Code != CodeCancelled {
		t.Fatalf("want cancelled, got %+v", verdict)
	}
	if clock.Now().After(t0.Add(10 * time.Second)) {
		t.Fatal("cancellation exceeded next tick")
	}
}

func TestEngineError(t *testing.T) {
	for _, operation := range []string{"list", "inspect"} {
		t.Run(operation, func(t *testing.T) {
			clock := newClock()
			engine := fakeEngine{
				list: func(context.Context, string, string) ([]Instance, error) {
					if operation == "list" {
						return nil, errors.New("engine failure")
					}
					return []Instance{{ID: "a"}}, nil
				},
				inspect: func(context.Context, string) (Container, error) { return Container{}, errors.New("engine failure") },
			}
			expectFail(t, New(clock, engine).Observe(context.Background(), stackSet(t, 1, false), clock.Now()))
		})
	}
}
