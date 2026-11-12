// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original v14 container image command foundation; zero-effect fallback for missing ports. M24 zero-effect refusal for missing observed executor ports.

package activity

import (
	"context"

	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
)

func runtimeContainerImageOperation(op RuntimeOperation) bool {
	return op == RuntimeUpdateContainer || op == RuntimeRestoreContainerImage
}

func (s *ActivityService) AdmitPolicyContainerUpdate(ctx context.Context, env, target, key string, binding dto.PolicyBinding) (RuntimeAdmissionResult, error) {
	if s == nil || s.runtimeHTTP == nil {
		return RuntimeAdmissionResult{}, ErrRuntimeNotReady
	}
	admitted, _, err := s.runtimeHTTP.admit(ctx, env, key, RuntimeUpdateContainer, RuntimeTarget{Kind: "container", ID: target}, RuntimeOptions{ContainerImage: &dto.ContainerImageRequest{Policy: &binding}})
	return admitted, err
}

// Missing container ports refuse before any effect, including observed executions.
func unavailableContainerImage(ctx context.Context, command RuntimeCommand) error {
	result := dto.LifecycleResult{RunID: command.ProposedRunID, Code: "runtime.target-unavailable", HTTPStatus: 503}
	if dto.ObservedExecutionFromContext(ctx).Mode != "" {
		result.Recovery = &dto.RecoveryResult{Outcome: "not-needed"}
	}
	dto.RecordLifecycleResult(ctx, result)
	return imageDTO.Failure("runtime.target-unavailable", 503)
}
