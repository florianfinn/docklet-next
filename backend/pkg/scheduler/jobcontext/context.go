// Source: getarcaneapp/arcane/backend/pkg/scheduler/jobcontext/context.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

// Package jobcontext carries durable execution progress through domain operations.
package jobcontext

import (
	"context"

	schedulertypes "github.com/florianfinn/docklet-next/types/scheduler"
)

type (
	progressKeyInternal struct{}
	runKeyInternal      struct{}
)

func WithExecution(ctx context.Context, run schedulertypes.Run, persist func(schedulertypes.TargetOutcome) error) context.Context {
	ctx = context.WithValue(ctx, runKeyInternal{}, run)
	if persist != nil {
		ctx = context.WithValue(ctx, progressKeyInternal{}, persist)
	}
	return ctx
}

func Progress(ctx context.Context, target schedulertypes.TargetOutcome) error {
	if persist, ok := ctx.Value(progressKeyInternal{}).(func(schedulertypes.TargetOutcome) error); ok {
		return persist(target)
	}
	return nil
}

func Run(ctx context.Context) (schedulertypes.Run, bool) {
	run, ok := ctx.Value(runKeyInternal{}).(schedulertypes.Run)
	return run, ok
}

// ConfirmedTarget returns a terminal outcome only for a target whose completion
// was persisted or confirmed against its activity by the execution coordinator.
func ConfirmedTarget(run schedulertypes.Run, targetID string) schedulertypes.Outcome {
	for _, target := range run.Outcome.Targets {
		if target.ID == targetID && target.Status == schedulertypes.Succeeded {
			return schedulertypes.Outcome{Status: schedulertypes.Succeeded, ActivityID: target.ActivityID, Targets: run.Outcome.Targets}
		}
	}
	return schedulertypes.Outcome{Status: schedulertypes.Failed, Message: "The interrupted operation has no confirmed completion", Targets: run.Outcome.Targets}
}
