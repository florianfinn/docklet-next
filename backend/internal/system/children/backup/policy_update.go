// Source: getarcaneapp/arcane/backend/internal/system/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Hub archive policy validation distinguishes client tools from storage.

package backup

import (
	"context"

	sharedbackup "github.com/florianfinn/docklet-next/backend/internal/backup"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
)

func (s *Service) UpdatePolicies(ctx context.Context, updates []backuptypes.UpdateSystemBackupPolicy) (*backuptypes.SystemBackupPolicyCollection, error) {
	build := func(ctx context.Context, policy *backuptypes.SystemBackupPolicy, update backuptypes.UpdateSystemBackupPolicy) error {
		if update.TargetID != "" {
			return ErrSystemBackupTargetUnavailable
		}
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
		policy.TargetID = ""
		return nil
	}
	if err := s.store.ReconcilePolicies(ctx, updates, build, s.jobs.Unregister, s.reschedulePolicy); err != nil {
		return nil, err
	}
	return s.GetPolicies(ctx)
}
