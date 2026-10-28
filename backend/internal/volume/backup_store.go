// Source: getarcaneapp/arcane/backend/internal/volume/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; split inventory reads; Explicit candidate prune execution; M12 backup foundation; split backup persistence adapter.

package volume

import (
	"context"
	"errors"
	"fmt"
	volumebackup "github.com/florianfinn/docklet-next/backend/internal/volume/children/backup"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"go.getarcane.app/kit/pkg"
	"gorm.io/gorm"
	"strings"
)

func (s *VolumeService) backupStoreInternal() volumebackup.Store {
	return volumebackup.Store{
		Create: func(ctx context.Context, record *volumetypes.Backup) error {
			entry := volumeBackupFromRecordInternal(record)
			if err := s.db.WithContext(ctx).Create(entry).Error; err != nil {
				return err
			}
			record.ID, record.UpdatedAt = entry.ID, entry.UpdatedAt
			return nil
		},
		Save: func(ctx context.Context, record *volumetypes.Backup) error {
			entry := volumeBackupFromRecordInternal(record)
			if err := s.db.WithContext(ctx).Save(entry).Error; err != nil {
				return err
			}
			record.UpdatedAt = entry.UpdatedAt
			return nil
		},
		Fail: func(ctx context.Context, backupID, message string) error {
			return s.db.WithContext(ctx).Model(&VolumeBackup{}).Where("id = ?", backupID).Updates(map[string]any{"status": VolumeBackupStatusFailed, "error": message}).Error
		},
		Delete: func(ctx context.Context, backupID string) error {
			return s.db.WithContext(ctx).Where("id = ?", backupID).Delete(&VolumeBackup{}).Error
		},
		Run: func(ctx context.Context, backupID string) (*volumetypes.Backup, error) {
			var entry VolumeBackup
			if err := s.db.WithContext(ctx).Where("id = ?", backupID).First(&entry).Error; err != nil {
				return nil, err
			}
			return entry.recordInternal(), nil
		},
		Runs: func(ctx context.Context, backupIDs []string) ([]*volumetypes.Backup, error) {
			var entries []VolumeBackup
			if err := s.db.WithContext(ctx).Where("id IN ?", backupIDs).Find(&entries).Error; err != nil {
				return nil, err
			}
			records := make([]*volumetypes.Backup, 0, len(entries))
			for i := range entries {
				records = append(records, entries[i].recordInternal())
			}
			return records, nil
		},
		Latest: func(ctx context.Context, policyID string) (*volumetypes.Backup, error) {
			var entry VolumeBackup
			err := s.db.WithContext(ctx).Where("policy_id = ?", policyID).Order("created_at DESC").First(&entry).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			return entry.recordInternal(), nil
		},
		List: s.listBackupsInternal,
	}
}

// listBackupsInternal pages one volume's backup history; both or neither management type selected means no type filter.
func (s *VolumeService) listBackupsInternal(ctx context.Context, volumeName string, params pagination.QueryParams) ([]volumetypes.Backup, int64, error) {
	query := s.db.WithContext(ctx).Model(&VolumeBackup{}).Where("volume_name = ?", volumeName)
	var system, managed bool
	for value := range strings.SplitSeq(params.Filters["type"], ",") {
		switch backuptypes.ManagementType(strings.TrimSpace(value)) {
		case backuptypes.ManagementTypeSystem:
			system = true
		case backuptypes.ManagementTypeVolume:
			managed = true
		}
	}
	if system != managed {
		if system {
			query = query.Where("policy_id LIKE ?", backuptypes.SystemVolumePolicyPrefix+"%")
		} else {
			query = query.Where("policy_id NOT LIKE ? OR policy_id IS NULL", backuptypes.SystemVolumePolicyPrefix+"%")
		}
	}

	if params.Search != "" {
		pattern := "%" + params.Search + "%"
		query = query.Where(
			"id LIKE ? OR status LIKE ? OR trigger LIKE ? OR destination LIKE ? OR COALESCE(local_snapshot_id, "+
				"'') LIKE ? OR COALESCE(remote_snapshot_id, '') LIKE ? OR COALESCE(error, '') LIKE ?",
			pattern,
			pattern,
			pattern,
			pattern,
			pattern,
			pattern,
			pattern,
		)
	}

	var totalItems int64
	if err := query.Count(&totalItems).Error; err != nil {
		return nil, 0, err
	}

	sortCol := "created_at"
	sortOrder := "DESC"
	if params.Sort != "" {
		switch params.Sort {
		case "createdAt", "created_at":
			sortCol = "created_at"
		case "id":
			sortCol = "id"
		case "size":
			sortCol = "size"
		case "status":
			sortCol = "status"
		case "trigger":
			sortCol = "trigger"
		case "destination":
			sortCol = "destination"
		case "remoteSnapshotId", "remote_snapshot_id":
			sortCol = "remote_snapshot_id"
		default:
			sortCol = "created_at"
		}

		sortOrder = kit.Ternary(params.Order == pagination.SortDesc, "DESC", "ASC")
	}
	query = query.Order(fmt.Sprintf("%s %s", sortCol, sortOrder))

	if params.Limit > 0 {
		query = query.Offset(params.Start).Limit(params.Limit)
	}

	var entries []VolumeBackup
	if err := query.Find(&entries).Error; err != nil {
		return nil, 0, err
	}
	records := make([]volumetypes.Backup, 0, len(entries))
	for i := range entries {
		record := entries[i].recordInternal()
		record.Type = backuptypes.ManagementTypeForPolicy(record.PolicyID)
		records = append(records, *record)
	}
	return records, totalItems, nil
}
