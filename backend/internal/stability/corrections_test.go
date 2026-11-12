// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original proofs for the first-round review corrections of the observer.

package stability

import (
	"context"
	"testing"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
)

// R1S2-1: an extra running replica breaks the frozen cardinality.
func TestExtraReplicaFails(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	engine := fakeEngine{
		list:    func(context.Context, string, string) ([]Instance, error) { return []Instance{{ID: "a"}, {ID: "b"}}, nil },
		inspect: func(_ context.Context, id string) (Container, error) { return running(id, t0), nil },
	}
	expectFail(t, New(clock, engine).Observe(context.Background(), stackSet(t, 1, false), t0))
}

// R1S2-2 (AUTO-5): a container first seen after the first sample fails at once.
func TestLateContainerFails(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	engine := fakeEngine{
		list: func(context.Context, string, string) ([]Instance, error) {
			if clock.Now().Sub(t0) >= 60*time.Second {
				return []Instance{{ID: "a"}, {ID: "b"}}, nil
			}
			return []Instance{{ID: "a"}}, nil
		},
		inspect: func(_ context.Context, id string) (Container, error) { return running(id, t0), nil },
	}
	expectFail(t, New(clock, engine).Observe(context.Background(), stackSet(t, 2, false), t0))
	if clock.Now().Sub(t0) != 60*time.Second {
		t.Fatal("late container did not fail when it appeared")
	}
}

// R1S2-3: a container target carrying the one-off label is still observed and passes.
func TestContainerTargetIgnoresOneOffLabel(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	engine := fakeEngine{inspect: func(_ context.Context, id string) (Container, error) {
		c := running(id, t0)
		c.OneOff = true
		return c, nil
	}}
	expectPass(t, New(clock, engine).Observe(context.Background(), NewContainerSet("a"), t0), clock, t0)
}

// R1S2-4: Docker's "none" health means no healthcheck.
func TestNoneHealthMeansNoHealthcheck(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	verdict := observeOne(clock, func(_ time.Duration, c *Container) { c.Health = "none" })
	expectPass(t, verdict, clock, t0)
}

// R1S2-8 (O-9a behavioural): a zero-scale service with a leftover exited container is
// not required beside a scaled service.
func TestZeroScaleLeftoverIsNotRequired(t *testing.T) {
	zero, one := 0, 1
	model := &compose.Project{Name: "example", Services: compose.Services{"app": {Name: "app", Scale: &one}, "worker": {Name: "worker", Scale: &zero}}}
	set, err := NewStackSet(model, []string{"app", "worker"})
	if err != nil {
		t.Fatal(err)
	}
	clock := newClock()
	t0 := clock.Now()
	engine := fakeEngine{
		list: func(_ context.Context, _, service string) ([]Instance, error) {
			if service == "worker" {
				return []Instance{{ID: "w"}}, nil
			}
			return []Instance{{ID: "a"}}, nil
		},
		inspect: func(_ context.Context, id string) (Container, error) {
			c := running(id, t0)
			if id == "w" {
				c.State = "exited"
			}
			return c, nil
		},
	}
	expectPass(t, New(clock, engine).Observe(context.Background(), set, t0), clock, t0)
}
