// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Trusted scheduler uses the same preflight, admission and receipt owner as manual updates; M24 bounded re-query of held scheduled runs.

package activity

import (
	"context"
	"errors"
	"time"

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
	if err := s.db.WithContext(ctx).First(&row, "run_id = ? AND environment_id = ?", run, env).Error; err != nil {
		return RuntimeRunReference{}, err
	}
	return RuntimeRunReference{Store: RuntimeStoreBinding{EnvironmentID: row.EnvironmentID, RegistrationID: row.RegistrationID, ManagerID: row.ManagerID, StoreGeneration: row.StoreGeneration, ExecutorGeneration: row.ExecutorGeneration}, RunID: row.RunID}, nil
}

// A scheduled window keeps its bound run's held transport or continuity unknown
// re-queried through the bounded Refresh until the hold cap, so it settles once
// the executor is reachable again. It never resends the effect; after the cap the
// result stays unknown and the job Retry reconciles from the receipt.
const (
	policyHoldCap     = 10 * time.Minute
	policyHoldSpacing = 30 * time.Second
)

func (s *ActivityService) WaitPolicyUpdate(ctx context.Context, env, run string) (activityDTO.RuntimeSnapshot, error) {
	ref, err := s.policyReference(ctx, env, run)
	if err != nil {
		return activityDTO.RuntimeSnapshot{}, err
	}
	snapshot, err := s.runtimeHTTP.wait(ctx, ref, nil)
	remote := s.runtimeHTTP.service.remote
	if err != nil || ref.Store.EnvironmentID == "0" || remote == nil {
		return snapshot, err
	}
	deadline := remote.schedule.Now().Add(policyHoldCap)
	for {
		if snapshot.OutcomeState != activityDTO.RuntimeOutcomeUnknown {
			if isTerminalActivityStatusInternal(snapshot.Status) {
				return snapshot, nil
			}
			// A refresh proved the run still active: resume the ordinary wait.
			if snapshot, err = s.runtimeHTTP.wait(ctx, ref, nil); err != nil {
				return snapshot, err
			}
			continue
		}
		if !s.policyHoldRetryable(ctx, run) || !remote.schedule.Now().Before(deadline) {
			return snapshot, nil
		}
		if err := remote.schedule.Wait(ctx, policyHoldSpacing); err != nil {
			return snapshot, err
		}
		if refreshed, err := remote.Refresh(ctx, ref); err == nil {
			snapshot = refreshed
		} else if ctx.Err() != nil {
			return snapshot, ctx.Err()
		}
	}
}

// Only holds the executor may still resolve are re-queried; expired or invalid
// evidence and administrator holds keep today's explicit recovery.
func (s *ActivityService) policyHoldRetryable(ctx context.Context, run string) bool {
	var row RuntimeReceipt
	if err := s.db.WithContext(ctx).Select("unknown_reason", "hub_disposition").First(&row, "run_id = ?", run).Error; err != nil {
		return false
	}
	return row.HubDisposition == nil && row.UnknownReason != nil && (*row.UnknownReason == "transport" || *row.UnknownReason == "process-continuity")
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
