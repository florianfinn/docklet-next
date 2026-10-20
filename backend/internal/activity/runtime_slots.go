// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Runtime composition of the existing activity limiter and durable phases.

package activity

func runtimeQueuesInternal(operation RuntimeOperation) bool {
	return operation == RuntimeDeployProject || operation == RuntimeRedeployProject
}

// Each phase checks durable intent and the owned context under runningMu. The
// limiter wait itself holds no owner/publish lock or database transaction.
func (s *RuntimeService) acquireLocalSlotInternal(owner *runtimeOwnerInternal) (bool, error) {
	ready, err := s.localPhaseInternal(owner, false)
	if err != nil || !ready {
		return false, err
	}
	if runtimeQueuesInternal(owner.command.command.Operation) {
		if owner.ctx.Err() != nil {
			return false, nil
		}
		release, err := s.activity.limiter.acquireInternal(owner.ctx, owner.id.EnvironmentID)
		if err != nil {
			return false, err
		}
		// Even a grant racing with cancellation belongs to terminal settlement.
		// Generic AwaitActivitySlot releases on write/cancel failure; runtime must
		// retain capacity with its frozen result until terminal COMMIT succeeds.
		s.activity.registerOwnedSlotReleaseInternal(owner.id.RunID, release, true)
	}
	// The limiter can grant an already cancelled context when capacity is free.
	// Never use a grant as the effect decision: recheck durable intent/context.
	return s.localPhaseInternal(owner, true)
}
