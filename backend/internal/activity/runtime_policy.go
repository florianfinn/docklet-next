// Source: docklet-next/docs/design/update-policies.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Trusted scheduler uses the same preflight, admission and receipt owner as manual updates.

package activity

import (
	"context"
	"errors"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"gorm.io/gorm"
)

func (s *ActivityService) AdmitPolicyUpdate(ctx context.Context, env, target, key string, request dto.LifecycleRequest, policy dto.PolicyBinding) (RuntimeAdmissionResult, error) {
	if s == nil || s.runtimeHTTP == nil {
		return RuntimeAdmissionResult{}, ErrRuntimeNotReady
	}
	admitted, _, err := s.runtimeHTTP.admit(ctx, env, key, RuntimeUpdateProject, RuntimeTarget{Kind: "stack", ID: target}, RuntimeOptions{Lifecycle: &dto.LifecycleIntent{Request: request, Policy: &policy}})
	return admitted, err
}
func (s *ActivityService) policyReference(ctx context.Context, env, run string) (RuntimeRunReference, error) {
	if s == nil || s.runtimeHTTP == nil {
		return RuntimeRunReference{}, ErrRuntimeNotReady
	}
	var row RuntimeReceipt
	if err := s.db.WithContext(ctx).First(&row, "run_id = ? AND environment_id = ?", run, env).Error
 err != nil {
		return RuntimeRunReference{}, err
	}
	return RuntimeRunReference{Store: RuntimeStoreBinding{EnvironmentID: row.EnvironmentID, RegistrationID: row.RegistrationID, ManagerID: row.ManagerID, StoreGeneration: row.StoreGeneration, ExecutorGeneration: row.ExecutorGeneration}, RunID: row.RunID}, nil
}
func (s *ActivityService) WaitPolicyUpdate(ctx context.Context, env, run string) (activityDTO.RuntimeSnapshot, error) {
	ref, err := s.policyReference(ctx, env, run)
	if err != nil {
		return activityDTO.RuntimeSnapshot{}, err
	}
	return s.runtimeHTTP.wait(ctx, ref, nil)
}

// PolicyUpdateSnapshot is receipt-only recovery. It cannot dispatch, preflight or
// create a runtime run, including when a checkpoint predates admission.
func (s *ActivityService) PolicyUpdateSnapshot(ctx context.Context, env, run string) (activityDTO.RuntimeSnapshot, bool, error) {
	ref, err := s.policyReference(ctx, env, run)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return activityDTO.RuntimeSnapshot{}, false, nil
	}
	if err != nil {
		return activityDTO.RuntimeSnapshot{}, false, err
	}
	snapshot, err := s.runtimeHTTP.service.boundSnapshotInternal(ctx, ref)
	return snapshot, true, err
}
