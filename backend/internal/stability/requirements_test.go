// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original frozen membership and exclusion tests.

package stability

import (
	"context"
	"testing"

	compose "github.com/compose-spec/compose-go/v2/types"
)

func TestO9aZeroScale(t *testing.T) {
	set := stackSet(t, 0, false)
	if len(set.requirements) != 0 {
		t.Fatal("zero scale was included")
	}
	clock := newClock()
	if verdict := New(clock, fakeEngine{}).Observe(context.Background(), set, clock.Now()); !verdict.Pass {
		t.Fatalf("empty set: %+v", verdict)
	}
}

func TestO9bCompletionNotContinuous(t *testing.T) {
	set := stackSet(t, 2, true)
	if len(set.requirements) != 1 || set.requirements[0].kind != CompletionJob || set.requirements[0].want != 2 {
		t.Fatalf("completion dependency included in continuous set: %+v", set)
	}
}

func TestO9cProvenOneOff(t *testing.T) {
	for _, labelSource := range []string{"list", "inspect"} {
		t.Run(labelSource, func(t *testing.T) {
			clock := newClock()
			t0 := clock.Now()
			engine := fakeEngine{
				list: func(context.Context, string, string) ([]Instance, error) {
					return []Instance{{ID: "a"}, {ID: "oneoff", OneOff: labelSource == "list"}}, nil
				},
				inspect: func(_ context.Context, id string) (Container, error) {
					if id == "oneoff" {
						return Container{ID: id, State: "exited", ExitCode: 1, OneOff: true}, nil
					}
					return running(id, t0), nil
				},
			}
			expectPass(t, New(clock, engine).Observe(context.Background(), stackSet(t, 1, false), t0), clock, t0)
		})
	}
}

func TestFrozenComposeRequirements(t *testing.T) {
	scale := 3
	replicas := 2
	model := &compose.Project{Name: "example", Services: compose.Services{
		"scaled":       {Name: "scaled", Scale: &scale, Deploy: &compose.DeployConfig{Replicas: &replicas}},
		"replicated":   {Name: "replicated", Deploy: &compose.DeployConfig{Replicas: &replicas}},
		"default":      {Name: "default"},
		"not-selected": {Name: "not-selected"},
	}}
	names := []string{"scaled", "replicated", "default", "scaled"}
	set, err := NewStackSet(model, names)
	if err != nil {
		t.Fatal(err)
	}
	scale, replicas = 8, 9
	model.Name = "changed"
	model.Services["default"] = compose.ServiceConfig{Scale: &scale}
	names[0] = "not-selected"
	if len(set.requirements) != 3 {
		t.Fatalf("want three unique services: %+v", set)
	}
	for i, want := range []int{1, 2, 3} {
		if got := set.requirements[i]; got.want != want || got.project != "example" || got.kind != Continuous {
			t.Fatalf("requirement changed with model: %+v", got)
		}
	}
}

func TestInvalidModel(t *testing.T) {
	negative := -1
	for _, model := range []*compose.Project{nil, {}, {Name: "example"}, {Name: "example", Services: compose.Services{"app": {Scale: &negative}}}} {
		if _, err := NewStackSet(model, []string{"app"}); err == nil {
			t.Fatal("invalid model accepted")
		}
	}
}

func TestEmptyContainerSet(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	verdict := New(clock, fakeEngine{}).Observe(context.Background(), NewContainerSet(""), t0)
	if !verdict.Pass || !clock.Now().Equal(t0) {
		t.Fatalf("stopped target did not pass without engine calls: %+v", verdict)
	}
}
