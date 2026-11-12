// Source: getarcaneapp/arcane/backend/internal/system/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Hub archive policy validation distinguishes client tools from storage; M16 bind mounts and backup completion.

package backup

import (
	"context"
	"sort"

	sharedbackup "github.com/florianfinn/docklet-next/backend/internal/backup"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
)

func (s *Service) UpdatePolicies(ctx context.Context, updates []backuptypes.UpdateSystemBackupPolicy) (*backuptypes.SystemBackupPolicyCollection, error) {
	for _, update := range updates {
		if err := s.validateCopyTarget(ctx, update.TargetID); err != nil {
			return nil, err
		}
	}
	build := func(ctx context.Context, policy *backuptypes.SystemBackupPolicy, update backuptypes.UpdateSystemBackupPolicy) error {

		normalized, err := sharedbackup.ValidatePolicyUpdate(ctx, "system", backuptypes.UpdateBackupPolicy{
			ID: update.ID, Enabled: update.Enabled, Schedule: update.Schedule,
			RetentionCount: update.RetentionCount, LocalEnabled: true})
		if err != nil {
			return err
		}
		if update.Enabled {
			if err := s.databaseProviderError(); err != nil {
				return err
			}
		}
		policy.Enabled, policy.Schedule, policy.RetentionCount = update.Enabled, normalized.Schedule, normalized.RetentionCount
		policy.TargetID = update.TargetID
		return nil
	}
	reconcile := func() error {
		return s.store.ReconcilePolicies(ctx, updates, build, s.jobs.Unregister, s.reschedulePolicy)
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, update := range updates {
		if update.TargetID != "" && !seen[update.TargetID] {
			seen[update.TargetID] = true
			ids = append(ids, update.TargetID)
		}
	}
	sort.Strings(ids)
	var fenced func(int) error
	fenced = func(index int) error {
		if index == len(ids) {
			return reconcile()
		}
		id := ids[index]
		if id == "" {
			return fenced(index + 1)
		}
		if s.targets == nil {
			return ErrSystemBackupTargetUnavailable
		}
		return s.targets.WithActive(ctx, id, func() error { return fenced(index + 1) })
	}
	if err := fenced(0); err != nil {
		return nil, err
	}
	return s.GetPolicies(ctx)
}
