// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original persisted last-observed availability without page-view repository access.

package backup

import (
	"context"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/backup"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"gorm.io/gorm"
)

func observedBackupStatus(status, code string, format volume.BackupFormat, unavailable *time.Time) (string, string) {
	if format != volume.BackupFormatRustic || unavailable != nil {
		return "unavailable", "volume.backup.snapshot-not-found"
	}
	return status, code
}

func destinationRows(db *gorm.DB, destination string) *gorm.DB {
	db = db.Table("volume_backups").Where("format = ? AND local_snapshot_id <> ''", volume.BackupFormatRustic)
	if destination == "" || destination == "local" {
		return db.Where("destination IN ?", []string{"", "local"})
	}
	return db.Where("destination = ?", destination)
}

func (s *Service) observeRepository(repository backup.Repository, destination string) backup.Repository {
	repository.ObserveSnapshot = func(ctx context.Context, id string, found bool) error {
		var unavailable *time.Time
		if !found {
			now := time.Now().UTC()
			unavailable = &now
		}
		return destinationRows(s.deps.DB.WithContext(ctx), destination).Where("local_snapshot_id = ?", id).Update("snapshot_unavailable_at", unavailable).Error
	}
	repository.ObserveInventory = func(ctx context.Context, snapshots []backup.DiscoveredSnapshot) error {
		ids := make([]string, 0, len(snapshots))
		for _, snapshot := range snapshots {
			ids = append(ids, snapshot.ID)
		}
		return s.deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			missing := destinationRows(tx, destination)
			if len(ids) > 0 {
				if err := destinationRows(tx, destination).Where("local_snapshot_id IN ?", ids).Update("snapshot_unavailable_at", nil).Error; err != nil {
					return err
				}
				missing = missing.Where("local_snapshot_id NOT IN ?", ids)
			}
			return missing.Update("snapshot_unavailable_at", time.Now().UTC()).Error
		})
	}
	return repository
}

func entryInternal(entry *volume.Backup) volume.BackupEntry {
	status, code := observedBackupStatus(entry.Status, entry.ErrorCode, entry.Format, entry.SnapshotUnavailableAt)
	return volume.BackupEntry{
		ActivityID: entry.ActivityID, ID: entry.ID, VolumeName: entry.VolumeName, Size: entry.Size,
		CreatedAt: entry.CreatedAt.Format(time.RFC3339), Status: status, Trigger: entry.Trigger,
		Destination: entry.Destination, Format: entry.Format, LocalSnapshotID: entry.LocalSnapshotID,
		SnapshotUnavailableAt: entry.SnapshotUnavailableAt, PolicyID: entry.PolicyID, Error: entry.Error, ErrorCode: code, StopContainers: entry.StopContainers, Type: backuptypes.ManagementTypeForPolicy(entry.PolicyID),
	}
}

func (s *Service) latestRunInternal(ctx context.Context, policyID string) (*volume.BackupEntry, error) {
	entry, err := s.deps.Store.Latest(ctx, policyID)
	if err != nil || entry == nil {
		return nil, err
	}
	return new(entryInternal(entry)), nil
}
