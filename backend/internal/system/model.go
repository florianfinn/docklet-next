// Source: getarcaneapp/arcane/backend/internal/system/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations.

package system

import (
	"time"

	"github.com/florianfinn/docklet-next/types/backup"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

type SystemBackupStatus string

const (
	SystemBackupStatusRunning   SystemBackupStatus = backup.SystemBackupStatusRunning
	SystemBackupStatusSucceeded SystemBackupStatus = backup.SystemBackupStatusSucceeded
	SystemBackupStatusFailed    SystemBackupStatus = backup.SystemBackupStatusFailed

	SystemBackupTriggerManual    SystemBackupTrigger = backup.SystemBackupTriggerManual
	SystemBackupTriggerScheduled SystemBackupTrigger = backup.SystemBackupTriggerScheduled
	SystemBackupTriggerSafety    SystemBackupTrigger = backup.SystemBackupTriggerSafety
)

type SystemBackupTrigger string

type SystemBackupRun struct {
	database.BaseModel

	Size            int64                          `json:"size" gorm:"column:size" sortable:"true"`
	CreatedAt       time.Time                      `json:"createdAt" gorm:"column:created_at" sortable:"true"`
	Status          SystemBackupStatus             `json:"status" gorm:"column:status;type:text;not null" sortable:"true"`
	Trigger         SystemBackupTrigger            `json:"trigger" gorm:"column:trigger;type:text;not null" sortable:"true"`
	Destination     backup.SystemBackupDestination `json:"destination" gorm:"column:destination;type:text;not null" sortable:"true"`
	LocalSnapshotID string                         `json:"localSnapshotId,omitempty" gorm:"column:local_snapshot_id;type:text"`
	PolicyID        string                         `json:"policyId,omitempty" gorm:"column:policy_id;type:text;index"`
	Error           string                         `json:"error,omitempty" gorm:"column:error;type:text"`
}

func (SystemBackupRun) TableName() string { return "system_backup_runs" }

func (b SystemBackupRun) ToDTO() backup.SystemBackupRun {
	return backup.SystemBackupRun{
		ID: b.ID, Size: b.Size, CreatedAt: b.CreatedAt, Status: string(b.Status), Trigger: string(b.Trigger),
		Destination: b.Destination, LocalSnapshotID: b.LocalSnapshotID, PolicyID: b.PolicyID, Error: b.Error,
	}
}

func systemBackupRunFromDTOInternal(dto backup.SystemBackupRun) SystemBackupRun {
	run := SystemBackupRun{
		Size: dto.Size, CreatedAt: dto.CreatedAt, Status: SystemBackupStatus(dto.Status), Trigger: SystemBackupTrigger(dto.Trigger),
		Destination: dto.Destination, LocalSnapshotID: dto.LocalSnapshotID, PolicyID: dto.PolicyID, Error: dto.Error,
	}
	run.ID = dto.ID
	return run
}

type SystemBackupPolicy struct {
	database.BaseModel

	Enabled        bool   `gorm:"column:enabled;not null;default:false"`
	Schedule       string `gorm:"column:schedule;type:text;not null"`
	RetentionCount int    `gorm:"column:retention_count;not null;default:7"`
	LocalEnabled   bool   `gorm:"column:local_enabled;not null;default:true"`
}

func (SystemBackupPolicy) TableName() string { return "system_backup_policies" }

func (p SystemBackupPolicy) ToDTO() backup.SystemBackupPolicy {
	return backup.SystemBackupPolicy{
		ID: p.ID, Enabled: p.Enabled, Schedule: p.Schedule, RetentionCount: p.RetentionCount,
		LocalEnabled: p.LocalEnabled,
	}
}

func (p *SystemBackupPolicy) applyDTOInternal(dto backup.SystemBackupPolicy) {
	p.Enabled, p.Schedule, p.RetentionCount = dto.Enabled, dto.Schedule, dto.RetentionCount
	p.LocalEnabled = dto.LocalEnabled
}
