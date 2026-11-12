// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original bounded executor-side stability observer.

package stability

import (
	"context"
	"errors"
	"time"
)

type sample struct {
	startedAt    time.Time
	restartCount int
	done         bool
}

type membership struct {
	seen    map[string]sample
	count   int
	ready   bool
	sampled bool
}

func unstable(detail string) Verdict { return Verdict{Code: CodeUnstable, Detail: detail} }

func contextVerdict(ctx context.Context) Verdict {
	if errors.Is(ctx.Err(), context.Canceled) {
		return Verdict{Cancelled: true, Code: CodeCancelled, Detail: "Observation cancelled"}
	}
	return unstable("Observation budget exhausted")
}

// Observe starts after joined workload effects and identity verification. Capture
// t0 with Clock.Now at that boundary; deploy time never consumes this window.
// Sampling cannot prove unsampled transient events; restart freshness supplements it.
func (o *Observer) Observe(ctx context.Context, set RequiredSet, t0 time.Time) Verdict {
	if ctx.Err() != nil {
		return contextVerdict(ctx)
	}
	if len(set.requirements) == 0 {
		return Verdict{Pass: true}
	}
	end := t0.Add(Window)
	limit := end.Add(Grace)
	if outer, ok := ctx.Deadline(); ok && outer.Before(limit) {
		limit = outer
	}
	states := make([]membership, len(set.requirements))
	for i := range states {
		states[i].seen = map[string]sample{}
	}
	for {
		if ctx.Err() != nil {
			return contextVerdict(ctx)
		}
		if !o.clock.Now().Before(limit) {
			return unstable("Observation budget exhausted")
		}
		for i, required := range set.requirements {
			if detail := o.sample(ctx, required, &states[i], limit); detail != "" {
				if ctx.Err() != nil {
					return contextVerdict(ctx)
				}
				return unstable(detail)
			}
		}
		if ctx.Err() != nil {
			return contextVerdict(ctx)
		}
		if !o.clock.Now().Before(end) {
			for i, required := range set.requirements {
				if states[i].count != required.want {
					return unstable("Required replica count not reached")
				}
				if !states[i].ready {
					return unstable("Required containers not ready")
				}
			}
			return Verdict{Pass: true}
		}
		next := o.clock.Now().Add(Interval)
		if end.Before(next) {
			next = end
		}
		if limit.Before(next) {
			next = limit
		}
		if err := o.clock.Wait(ctx, next); err != nil {
			return contextVerdict(ctx)
		}
	}
}

func (o *Observer) sample(ctx context.Context, required requirement, state *membership, limit time.Time) string {
	instances := []Instance{{ID: required.id}}
	if required.id == "" {
		call, cancel := o.clock.WithDeadline(ctx, limit)
		listed, err := o.engine.List(call, required.project, required.service)
		failed := err != nil || call.Err() != nil || !o.clock.Now().Before(limit)
		cancel()
		if failed {
			return "Container listing failed or exceeded its deadline"
		}
		instances = listed
	}
	current := map[string]bool{}
	state.ready = true
	stack := required.id == ""
	for _, instance := range instances {
		// One-off exclusion is a stack membership rule; a container target is always required.
		if stack && instance.OneOff {
			continue
		}
		if instance.ID == "" || current[instance.ID] {
			return "Invalid container membership"
		}
		if previous, ok := state.seen[instance.ID]; ok && previous.done {
			current[instance.ID] = true
			continue
		}
		call, cancel := o.clock.WithDeadline(ctx, limit)
		container, err := o.engine.Inspect(call, instance.ID)
		failed := err != nil || call.Err() != nil || !o.clock.Now().Before(limit)
		cancel()
		if failed {
			return "Container inspection failed or exceeded its deadline"
		}
		if container.ID != instance.ID {
			return "Container identity changed"
		}
		if stack && container.OneOff {
			continue
		}
		current[instance.ID] = true
		previous, seen := state.seen[instance.ID]
		// After a forced recreate every replica exists at t0; a later ID was never observed for W.
		if !seen && state.sampled {
			return "Container appeared during observation"
		}
		if (!seen && container.RestartCount != 0) || (seen && container.RestartCount != previous.restartCount) {
			return "Required container restarted"
		}
		if seen && !container.StartedAt.Equal(previous.startedAt) {
			return "Required container start time changed"
		}
		if required.kind == CompletionJob && container.State == "exited" {
			if container.ExitCode != 0 {
				return "Completion job failed"
			}
			state.seen[instance.ID] = sample{startedAt: container.StartedAt, restartCount: container.RestartCount, done: true}
			continue
		}
		if container.State != "running" {
			return "Required container is not running"
		}
		health := container.Health
		if health == "none" {
			health = ""
		}
		if health == "unhealthy" {
			return "Required container is unhealthy"
		}
		if required.kind == CompletionJob || (health != "" && health != "healthy") {
			state.ready = false
		}
		state.seen[instance.ID] = sample{startedAt: container.StartedAt, restartCount: container.RestartCount}
	}
	for id, previous := range state.seen {
		if !current[id] && !previous.done {
			return "Required container identity disappeared or changed"
		}
		if previous.done {
			current[id] = true
		}
	}
	state.count = len(current)
	state.sampled = true
	return ""
}
