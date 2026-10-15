// Source: getarcaneapp/arcane/backend/internal/system/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; PostgreSQL archive persistence replaces system recovery snapshots.

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

	Size              int64                        `json:"size" gorm:"column:size" sortable:"true"`
	CreatedAt         time.Time                    `json:"createdAt" gorm:"column:created_at" sortable:"true"`
	Status            SystemBackupStatus           `json:"status" gorm:"column:status;type:text;not null" sortable:"true"`
	Trigger           SystemBackupTrigger          `json:"trigger" gorm:"column:trigger;type:text;not null" sortable:"true"`
	ArchiveName       *string                      `json:"archiveName,omitempty" gorm:"column:archive_name;type:text"`
	ActivityID        string                       `json:"activityId,omitempty" gorm:"column:activity_id;type:text"`
	CompletedAt       *time.Time                   `json:"completedAt,omitempty" gorm:"column:completed_at"`
	SourceServerMajor *int                         `json:"sourceServerMajor,omitempty" gorm:"column:source_server_major"`
	MigrationVersion  *int64                       `json:"migrationVersion,omitempty" gorm:"column:migration_version"`
	CopyTargetID      string                       `json:"copyTargetId,omitempty" gorm:"column:copy_target_id;type:text"`
	CopyState         backup.SystemBackupCopyState `json:"copyState" gorm:"column:copy_state;type:text;not null;default:none"`
	CopyErrorCode     string                       `json:"copyErrorCode,omitempty" gorm:"column:copy_error_code;type:text"`
	ErrorCode         string                       `json:"errorCode,omitempty" gorm:"column:error_code;type:text"`
	PolicyID          string                       `json:"policyId,omitempty" gorm:"column:policy_id;type:text;index"`
}

func (SystemBackupRun) TableName() string { return "system_backup_runs" }

func (b SystemBackupRun) ToDTO() backup.SystemBackupRun {
	return backup.SystemBackupRun{
		ID: b.ID, Size: b.Size, CreatedAt: b.CreatedAt, Status: string(b.Status), Trigger: string(b.Trigger),
		ArchiveName: backupValueOrZeroInternal(b.ArchiveName), ActivityID: b.ActivityID, CompletedAt: b.CompletedAt,
		SourceServerMajor: backupValueOrZeroInternal(b.SourceServerMajor), MigrationVersion: backupValueOrZeroInternal(b.MigrationVersion),
		CopyTargetID: b.CopyTargetID, CopyState: b.CopyState, CopyErrorCode: b.CopyErrorCode,
		PolicyID: b.PolicyID, ErrorCode: b.ErrorCode,
	}
}

func systemBackupRunFromDTOInternal(dto backup.SystemBackupRun) SystemBackupRun {
	copyState := dto.CopyState
	if copyState == "" {
		copyState = backup.SystemBackupCopyNone
	}
	run := SystemBackupRun{
		Size: dto.Size, CreatedAt: dto.CreatedAt, Status: SystemBackupStatus(dto.Status), Trigger: SystemBackupTrigger(dto.Trigger),
		ArchiveName: backupOptionalValueInternal(dto.ArchiveName), ActivityID: dto.ActivityID, CompletedAt: dto.CompletedAt,
		SourceServerMajor: backupOptionalValueInternal(dto.SourceServerMajor), MigrationVersion: backupOptionalValueInternal(dto.MigrationVersion),
		CopyTargetID: dto.CopyTargetID, CopyState: copyState, CopyErrorCode: dto.CopyErrorCode,
		PolicyID: dto.PolicyID, ErrorCode: dto.ErrorCode,
	}
	run.ID = dto.ID
	return run
}

func backupOptionalValueInternal[T comparable](value T) *T {
	var zero T
	if value == zero {
		return nil
	}
	return &value
}

func backupValueOrZeroInternal[T any](value *T) T {
	if value == nil {
		var zero T
		return zero
	}
	return *value
}

type SystemBackupPolicy struct {
	database.BaseModel

	Enabled        bool   `gorm:"column:enabled;not null;default:false"`
	Schedule       string `gorm:"column:schedule;type:text;not null"`
	RetentionCount int    `gorm:"column:retention_count;not null;default:7"`
	TargetID       string `gorm:"column:target_id;type:text"`
}

func (SystemBackupPolicy) TableName() string { return "system_backup_policies" }

func (p SystemBackupPolicy) ToDTO() backup.SystemBackupPolicy {
	return backup.SystemBackupPolicy{
		ID: p.ID, Enabled: p.Enabled, Schedule: p.Schedule, RetentionCount: p.RetentionCount,
		TargetID: p.TargetID, LocalEnabled: true,
	}
}

func (p *SystemBackupPolicy) applyDTOInternal(dto backup.SystemBackupPolicy) {
	p.Enabled, p.Schedule, p.RetentionCount = dto.Enabled, dto.Schedule, dto.RetentionCount
	p.TargetID = dto.TargetID
}
