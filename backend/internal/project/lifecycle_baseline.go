// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Successful normal deploy baseline capture preserves truthful deploy outcome. M18 baseline fields merge with durable deploy-point evidence. M34 #548: recorded Compose environment inputs. M35 #555: external drift reports current revisions.

package project

import (
	"context"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	dto "github.com/florianfinn/docklet-next/types/updater"
)

func (s *ProjectService) captureDeployedBaseline(ctx context.Context, id string, snapshot *life.Snapshot) {
	execution := dto.ExecutionIdentityFromContext(ctx)
	if execution.RunID == "" || execution.RegistrationID == "" {
		return
	}
	result := s.deployPointResult(ctx, execution.RunID)
	result.BaselineCode = "update.baseline-unavailable"
	if snapshot != nil {
		snapshot.RunID = execution.RunID
		snapshot.RegistrationID = execution.RegistrationID
		if s.lifecycleObserve(ctx, snapshot) == nil && s.lifecycleStore().SaveBaseline(ctx, snapshot) == nil {
			result.BaselineCode = ""
			result.Verified = true
		}
	}
	s.recordLifecycleResult(ctx, execution.RunID, &result)
}

func (s *ProjectService) ImageBaselineStatus(ctx context.Context, id, registration string) (dto.BaselineStatus, error) {
	ctx = s.composeInputContext(ctx)
	externalDrift := false
	if p, pErr := s.getMutableProject(ctx, id); pErr == nil {
		baseline, _, e := s.lifecycleStore().Baseline(ctx, &life.Snapshot{TargetID: id, TargetIdentity: targetLifecycleIdentity(p), RegistrationID: registration})
		externalDrift = e == nil && s.checkLifecycleExternal(ctx, baseline) != nil
	}
	// Unreadable current inputs answer with lifecycleCurrent's declared failure.
	_, current, err := s.lifecycleCurrent(ctx, id, registration)
	if err != nil {
		return dto.BaselineStatus{}, err
	}
	status := dto.BaselineStatus{TargetIdentity: current.TargetIdentity, AutoExcluded: current.AutoExcluded, SavedRevision: current.SavedRevision, ObservedRevision: lifecycleObserved(current)}
	if externalDrift {
		// External inputs changed since the deploy: a valid status with current revisions, never available.
		status.Code = "update.config-drift"
		return status, nil
	}
	baseline, _, err := s.lifecycleStore().Baseline(ctx, current)
	if err != nil {
		status.Code = "update.baseline-unavailable"
		return status, nil
	}
	status.Available = baseline.SavedRevision == current.SavedRevision && lifecycleConfiguration(baseline) == lifecycleConfiguration(current)
	status.SourceRunID = baseline.RunID
	if !status.Available {
		status.Code = "update.baseline-unavailable"
		if baseline.SavedRevision != current.SavedRevision {
			status.Code = "update.config-drift"
		}
	}
	return status, nil
}
