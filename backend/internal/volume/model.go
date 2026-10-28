// Source: getarcaneapp/arcane/backend/internal/volume/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; persisted lifecycle responsibility; M12 backup foundation; M12 persisted volume lifecycle and legacy restore cut.

package volume

import (
	"encoding/json"
	"time"

	"github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

type VolumeBackupStatus string

const (
	VolumeBackupStatusRunning   VolumeBackupStatus = backup.VolumeBackupStatusRunning
	VolumeBackupStatusSucceeded VolumeBackupStatus = backup.VolumeBackupStatusSucceeded
	VolumeBackupStatusFailed    VolumeBackupStatus = backup.VolumeBackupStatusFailed

	VolumeBackupTriggerManual    VolumeBackupTrigger = backup.VolumeBackupTriggerManual
	VolumeBackupTriggerScheduled VolumeBackupTrigger = backup.VolumeBackupTriggerScheduled
	VolumeBackupTriggerSafety    VolumeBackupTrigger = backup.VolumeBackupTriggerSafety

	VolumeBackupFormatRustic VolumeBackupFormat = VolumeBackupFormat(volume.BackupFormatRustic)
)

type VolumeBackupTrigger string

type VolumeBackupFormat string

type VolumeBackup struct {
	database.BaseModel

	VolumeName            string                   `json:"volumeName" gorm:"column:volume_name;index"`
	Size                  int64                    `json:"size" gorm:"column:size"`
	CreatedAt             time.Time                `json:"createdAt" gorm:"column:created_at"`
	Status                VolumeBackupStatus       `json:"status" gorm:"column:status;type:text;not null;default:succeeded"`
	Trigger               VolumeBackupTrigger      `json:"trigger" gorm:"column:trigger;type:text;not null;default:manual"`
	Destination           volume.BackupDestination `json:"destination" gorm:"column:destination;type:text;not null;default:local"`
	Format                VolumeBackupFormat       `json:"format" gorm:"column:format;type:text;not null;default:archive"`
	SnapshotUnavailableAt *time.Time               `json:"snapshotUnavailableAt,omitempty"`
	LocalSnapshotID       string                   `json:"localSnapshotId,omitempty" gorm:"column:local_snapshot_id;type:text"`
	PolicyID              string                   `json:"policyId,omitempty" gorm:"column:policy_id;type:text;index"`
	Error                 string                   `json:"error,omitempty" gorm:"column:error;type:text"`
	StopContainers        bool                     `json:"stopContainers" gorm:"not null;default:false"`
	StoppedContainers     *string                  `json:"-"`
	ErrorCode             string                   `json:"errorCode,omitempty"`
	ActivityID            *string                  `json:"activityId,omitempty" gorm:"column:activity_id;type:text"`
	Type                  backup.ManagementType    `json:"type" gorm:"-"`
}

func (*VolumeBackup) TableName() string {
	return "volume_backups"
}

func (b *VolumeBackup) recordInternal() *volume.Backup {
	var stopped []volume.StoppedContainer
	if b.StoppedContainers != nil {
		_ = json.Unmarshal([]byte(*b.StoppedContainers), &stopped)
	}
	return &volume.Backup{
		ID: b.ID, UpdatedAt: b.UpdatedAt, VolumeName: b.VolumeName, Size: b.Size, CreatedAt: b.CreatedAt,
		Status: string(b.Status), Trigger: string(b.Trigger), Destination: b.Destination, Format: volume.BackupFormat(b.Format),
		SnapshotUnavailableAt: b.SnapshotUnavailableAt, LocalSnapshotID: b.LocalSnapshotID, PolicyID: b.PolicyID, Error: b.Error,
		ActivityID: b.ActivityID, Type: b.Type, StopContainers: b.StopContainers, StoppedContainers: stopped, ErrorCode: b.ErrorCode,
	}
}

func volumeBackupFromRecordInternal(record *volume.Backup) *VolumeBackup {
	var stopped *string
	if record.StoppedContainers != nil {
		value, _ := json.Marshal(record.StoppedContainers)
		stopped = new(string(value))
	}
	entry := &VolumeBackup{
		VolumeName: record.VolumeName, Size: record.Size, CreatedAt: record.CreatedAt, Status: VolumeBackupStatus(record.Status),
		Trigger: VolumeBackupTrigger(record.Trigger), Destination: record.Destination, Format: VolumeBackupFormat(record.Format),
		SnapshotUnavailableAt: record.SnapshotUnavailableAt, LocalSnapshotID: record.LocalSnapshotID, PolicyID: record.PolicyID,
		Error: record.Error, ActivityID: record.ActivityID, Type: record.Type, StopContainers: record.StopContainers, StoppedContainers: stopped, ErrorCode: record.ErrorCode,
	}
	entry.ID, entry.UpdatedAt = record.ID, record.UpdatedAt
	return entry
}
