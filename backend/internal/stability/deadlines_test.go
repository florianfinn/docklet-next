// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original deterministic deadline, error and outer-budget tests.

package stability

import (
	"context"
	"testing"
	"time"
)

func TestO6EngineDeadline(t *testing.T) {
	for _, operation := range []string{"list", "inspect"} {
		t.Run(operation, func(t *testing.T) {
			clock := newClock()
			t0 := clock.Now()
			entered := make(chan context.Context)
			release := make(chan struct{})
			result := make(chan Verdict, 1)
			block := func(ctx context.Context) {
				entered <- ctx
				<-release
			}
			engine := fakeEngine{
				list: func(ctx context.Context, _, _ string) ([]Instance, error) {
					if operation == "list" {
						block(ctx)
					}
					return []Instance{{ID: "a"}}, nil
				},
				inspect: func(ctx context.Context, id string) (Container, error) { block(ctx); return running(id, t0), nil },
			}
			set := stackSet(t, 1, false)
			go func() { result <- New(clock, engine).Observe(context.Background(), set, t0) }()
			call := <-entered
			deadline, present := call.Deadline()
			deadlineOK := present && !deadline.After(t0.Add(Window+Grace))
			// Independent release makes dropping a deadline an assertion failure, never a hang.
			clock.advance(t0.Add(Window + Grace))
			expired := false
			select {
			case <-call.Done():
				expired = call.Err() == context.DeadlineExceeded
			default:
			}
			close(release)
			verdict := <-result
			if !expired {
				t.Error("engine context did not expire on the fake clock")
			}
			if !deadlineOK {
				t.Errorf("engine deadline missing or exceeds observation bound: present=%v", present)
			}
			expectFail(t, verdict)
			if clock.Now().After(t0.Add(Window + Grace)) {
				t.Fatal("observer exceeded grace")
			}
		})
	}
}

func TestEveryEngineCallDeadline(t *testing.T) {
	for _, bounded := range []bool{false, true} {
		t.Run(map[bool]string{false: "window", true: "outer"}[bounded], func(t *testing.T) {
			clock := newClock()
			t0 := clock.Now()
			limit := t0.Add(Window + Grace)
			ctx := context.Background()
			if bounded {
				limit = t0.Add(17 * time.Second)
				var cancel context.CancelFunc
				ctx, cancel = clock.WithDeadline(ctx, limit)
				defer cancel()
			}
			lists, inspects := 0, 0
			check := func(ctx context.Context) {
				deadline, present := ctx.Deadline()
				if !present || !deadline.Equal(limit) {
					t.Fatal("engine call did not receive minimum deadline")
				}
			}
			engine := fakeEngine{
				list: func(ctx context.Context, _, _ string) ([]Instance, error) {
					check(ctx)
					lists++
					return []Instance{{ID: "a"}}, nil
				},
				inspect: func(ctx context.Context, id string) (Container, error) {
					check(ctx)
					inspects++
					return running(id, t0), nil
				},
			}
			verdict := New(clock, engine).Observe(ctx, stackSet(t, 1, false), t0)
			if bounded {
				expectFail(t, verdict)
				if !clock.Now().Equal(limit) {
					t.Fatal("outer budget not honoured")
				}
			} else {
				expectPass(t, verdict, clock, t0)
				if lists != 61 || inspects != 61 {
					t.Fatalf("want initial sample + 60 ticks, got %d/%d", lists, inspects)
				}
			}
		})
	}
}

func TestDeadlineExceededReturnedByEngine(t *testing.T) {
	clock := newClock()
	engine := fakeEngine{inspect: func(context.Context, string) (Container, error) { return Container{}, context.DeadlineExceeded }}
	expectFail(t, New(clock, engine).Observe(context.Background(), NewContainerSet("a"), clock.Now()))
}

func TestInspectIdentityMismatch(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	engine := fakeEngine{inspect: func(context.Context, string) (Container, error) { return running("other", t0), nil }}
	expectFail(t, New(clock, engine).Observe(context.Background(), NewContainerSet("a"), t0))
}

func TestMissingAndDuplicateMembership(t *testing.T) {
	for _, ids := range [][]Instance{nil, {{ID: "a"}, {ID: "a"}}, {{ID: ""}}} {
		clock := newClock()
		t0 := clock.Now()
		engine := fakeEngine{
			list:    func(context.Context, string, string) ([]Instance, error) { return ids, nil },
			inspect: func(_ context.Context, id string) (Container, error) { return running(id, t0), nil },
		}
		expectFail(t, New(clock, engine).Observe(context.Background(), stackSet(t, 1, false), t0))
	}
}

func TestCallerCancelDuringEngineCall(t *testing.T) {
	clock := newClock()
	ctx, cancel := context.WithCancel(context.Background())
	engine := fakeEngine{inspect: func(call context.Context, _ string) (Container, error) {
		cancel()
		<-call.Done()
		return Container{}, call.Err()
	}}
	verdict := New(clock, engine).Observe(ctx, NewContainerSet("a"), clock.Now())
	if verdict.Pass || !verdict.Cancelled || verdict.Code != CodeCancelled {
		t.Fatalf("want cancelled: %+v", verdict)
	}
}
