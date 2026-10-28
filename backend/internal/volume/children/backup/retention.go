// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original volume-scoped scheduled and pinned safety retention.

package backup

import (
	"context"
	"github.com/florianfinn/docklet-next/types/volume"
)

func (s *Service) retentionCandidates(ctx context.Context, name, current, trigger string, keep int) ([]*volume.Backup, error) {
	if keep <= 0 {
		return nil, nil
	}
	var rows []volume.Backup
	query := s.deps.DB.WithContext(ctx).Table("volume_backups").Where("volume_name = ? AND trigger = ? AND local_snapshot_id <> ''", name, trigger).Where("status IN ? OR id = ?", []string{"succeeded", "partial"}, current).Order("created_at DESC, id DESC")
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) <= keep {
		return nil, nil
	}
	var result []*volume.Backup
	for i := keep; i < len(rows); i++ {
		var pins int64
		if err := s.deps.DB.WithContext(ctx).Table("volume_restore_runs").Where("safety_backup_id = ? AND pending_restart = ?", rows[i].ID, true).Count(&pins).Error; err != nil {
			return nil, err
		}
		if pins == 0 {
			result = append(result, &rows[i])
		}
	}
	return result, nil
}

// Called only under the capturing/restoring operation's original ownership.
func (s *Service) applyVolumeBackupRetentionInternal(ctx context.Context, name, current string, keep int) error {
	entries, err := s.retentionCandidates(ctx, name, current, "scheduled", keep)
	if err != nil || len(entries) == 0 {
		return err
	}
	return s.deleteRusticBackupsInternal(ctx, entries, nil)
}
func (s *Service) pruneSafety(ctx context.Context, name string) error {
	entries, err := s.retentionCandidates(ctx, name, "", "safety", 2)
	if err != nil || len(entries) == 0 {
		return err
	}
	return s.deleteRusticBackupsInternal(ctx, entries, nil)
}
