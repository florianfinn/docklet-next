// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/children/policies/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations.

package policies

import (
	"github.com/florianfinn/docklet-next/types/volume"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

type VolumeBackupPolicy struct {
	database.BaseModel

	VolumeName     string `json:"volumeName" gorm:"column:volume_name;not null;index"`
	Enabled        bool   `json:"enabled" gorm:"column:enabled;not null;default:false"`
	Schedule       string `json:"schedule" gorm:"column:schedule;type:text;not null"`
	RetentionCount int    `json:"retentionCount" gorm:"column:retention_count;not null;default:7"`
	StopContainers bool   `json:"stopContainers" gorm:"column:stop_containers;not null;default:false"`
	LocalEnabled   bool   `json:"localEnabled" gorm:"column:local_enabled;not null"`
}

func (VolumeBackupPolicy) TableName() string {
	return "volume_backup_policies"
}

func (p VolumeBackupPolicy) ToDTO(lastRun *volume.BackupEntry) volume.BackupPolicy {
	return volume.BackupPolicy{
		ID:             p.ID,
		VolumeName:     p.VolumeName,
		Enabled:        p.Enabled,
		Schedule:       p.Schedule,
		RetentionCount: p.RetentionCount,
		StopContainers: p.StopContainers,
		LocalEnabled:   p.LocalEnabled,
		LastRun:        lastRun,
	}
}
