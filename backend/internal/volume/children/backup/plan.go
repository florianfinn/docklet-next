// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Split capture planning; immutable target identity and target-before-host-admission fencing.

package backup

import (
	"cmp"
	"context"

	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
)

type backupPlanInternal struct {
	policy                       *policies.VolumeBackupPolicy
	localEnabled, stopContainers bool
	destination                  volume.BackupDestination
}

func (s *Service) resolveBackupPlanInternal(ctx context.Context, name, trigger string, request volume.CreateBackupRequest, supplied *policies.VolumeBackupPolicy) (backupPlanInternal, error) {
	policy := supplied
	if trigger == dto.VolumeBackupTriggerSafety {
		return backupPlanInternal{localEnabled: true, destination: volume.BackupDestinationLocal}, nil
	}
	if policy == nil {
		var err error
		policy, err = s.policies.Policy(ctx, name, request.PolicyID)
		if err != nil {
			return backupPlanInternal{}, err
		}
		if policy == nil && request.PolicyID != "" {
			return backupPlanInternal{}, dto.NewProblem(422, "volume.backup.policy-invalid", nil)
		}
	}
	if policy != nil && !policy.LocalEnabled {
		return backupPlanInternal{}, dto.NewProblem(422, "volume.backup.policy-invalid", nil)
	}
	destination := cmp.Or(request.TargetID, string(request.Destination))
	if request.TargetID != "" && request.Destination != "" && request.TargetID != string(request.Destination) {
		return backupPlanInternal{}, dto.NewProblem(422, "volume.backup.policy-invalid", nil)
	}
	if destination == "" && policy != nil {
		destination = policy.TargetID
	}
	destination = cmp.Or(destination, "local")
	if destination != "local" {
		host, err := s.hostID(ctx)
		if err != nil {
			return backupPlanInternal{}, err
		}
		if _, err = s.targetStore.Resolve(ctx, destination, host, true); err != nil {
			return backupPlanInternal{}, err
		}
	}
	return backupPlanInternal{policy: policy, localEnabled: true, stopContainers: request.StopContainers || policy != nil && policy.StopContainers, destination: volume.BackupDestination(destination)}, nil
}

// Target fence comes before the host's policy/run admission mutex. Re-read the
// policy after taking both; a concurrent target switch retries with its new fence.
func (s *Service) prepareCapture(ctx context.Context, name, trigger string, request volume.CreateBackupRequest) (entry *volume.Backup, owner *operationOwnership, plan backupPlanInternal, err error) {
	for {
		if err = ctx.Err(); err != nil {
			return
		}
		var hint backupPlanInternal
		hint, err = s.resolveBackupPlanInternal(ctx, name, trigger, request, nil)
		if err != nil {
			return
		}
		changed := false
		err = s.withActiveTarget(ctx, string(hint.destination), func(ctx context.Context) error {
			s.policies.AdmissionLock()
			defer s.policies.AdmissionUnlock()
			var e error
			plan, e = s.resolveBackupPlanInternal(ctx, name, trigger, request, nil)
			if e != nil {
				return e
			}
			if plan.destination != hint.destination {
				changed = true
				return nil
			}
			policyID := request.PolicyID
			if plan.policy != nil {
				policyID = plan.policy.ID
			}
			entry, owner, e = s.prepareBackupInternal(ctx, name, trigger, policyID, plan)
			return e
		})
		if err != nil || !changed {
			return
		}
	}
}
