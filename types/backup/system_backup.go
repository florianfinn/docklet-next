// Source: getarcaneapp/arcane/types/backup/system_backup.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Replaced the system recovery wire contract with local PostgreSQL archives.

package backup

import (
	"errors"
	"time"
)

// SystemBackupDestination remains the volume-backup destination vocabulary.
type SystemBackupDestination string

const SystemBackupDestinationLocal SystemBackupDestination = "local"

const MaxSystemArchiveBytes int64 = 2 << 30

// ArchiveNameForID derives a server-owned filename from an opaque backup ID.
func ArchiveNameForID(id string) (string, error) {
	if len(id) < 1 || len(id) > 128 {
		return "", errors.New("invalid backup ID")
	}
	for _, char := range id {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_') {
			return "", errors.New("invalid backup ID")
		}
	}
	return id + ".dump", nil
}

const (
	SystemBackupStatusRunning   = "running"
	SystemBackupStatusSucceeded = "succeeded"
	SystemBackupStatusFailed    = "failed"

	SystemBackupTriggerManual    = "manual"
	SystemBackupTriggerScheduled = "scheduled"
	SystemBackupTriggerUploaded  = "uploaded"
	SystemBackupTriggerSafety    = "safety" // Volume-backup compatibility.
)

type SystemBackupCopyState string

const (
	SystemBackupCopyNone      SystemBackupCopyState = "none"
	SystemBackupCopyPending   SystemBackupCopyState = "pending"
	SystemBackupCopySucceeded SystemBackupCopyState = "succeeded"
	SystemBackupCopyFailed    SystemBackupCopyState = "failed"
)

// SystemBackupRun describes one local PostgreSQL custom-format archive.
type SystemBackupRun struct {
	ID                string                `json:"id"`
	ActivityID        string                `json:"activityId,omitempty"`
	ArchiveName       string                `json:"archiveName,omitempty"`
	Size              int64                 `json:"size" minimum:"0" maximum:"2147483648" doc:"Archive size in bytes"`
	CreatedAt         time.Time             `json:"createdAt"`
	CompletedAt       *time.Time            `json:"completedAt,omitempty"`
	Status            string                `json:"status" enum:"running,succeeded,failed"`
	Trigger           string                `json:"trigger" enum:"manual,scheduled,uploaded"`
	PolicyID          string                `json:"policyId,omitempty"`
	SourceServerMajor int                   `json:"sourceServerMajor,omitempty" minimum:"0"`
	MigrationVersion  int64                 `json:"migrationVersion,omitempty" minimum:"0"`
	CopyTargetID      string                `json:"copyTargetId,omitempty"`
	CopyState         SystemBackupCopyState `json:"copyState" enum:"none,pending,succeeded,failed"`
	CopyErrorCode     string                `json:"copyErrorCode,omitempty"`
	ErrorCode         string                `json:"errorCode,omitempty"`

	// Legacy fields remain only for the inactive Arcane engine until its replacement.
	Destination     SystemBackupDestination `json:"-"`
	LocalSnapshotID string                  `json:"-"`
	Error           string                  `json:"-"`
}

type SystemBackupPolicy struct {
	ID             string           `json:"id"`
	Enabled        bool             `json:"enabled"`
	Schedule       string           `json:"schedule"`
	RetentionCount int              `json:"retentionCount" minimum:"1"`
	TargetID       string           `json:"targetId,omitempty"`
	LastRun        *SystemBackupRun `json:"lastRun,omitempty"`
	LocalEnabled   bool             `json:"-"`
}

// UpdateBackupPolicy is shared with system-managed volume backup policies.
type UpdateBackupPolicy struct {
	ID             string `json:"id,omitempty"`
	Enabled        bool   `json:"enabled"`
	Schedule       string `json:"schedule"`
	RetentionCount int    `json:"retentionCount" minimum:"1"`
	StopContainers bool   `json:"stopContainers,omitempty"`
	LocalEnabled   bool   `json:"localEnabled"`
}

type UpdateSystemBackupPolicy struct {
	ID             string `json:"id,omitempty" pattern:"^[A-Za-z0-9_-]{1,128}$"`
	Enabled        bool   `json:"enabled"`
	Schedule       string `json:"schedule"`
	RetentionCount int    `json:"retentionCount" minimum:"1"`
	TargetID       string `json:"targetId,omitempty" pattern:"^[A-Za-z0-9_-]{1,128}$"`
	LocalEnabled   bool   `json:"-"`
}

type SystemBackupPolicyCollection struct {
	Policies          []SystemBackupPolicy `json:"policies"`
	RecoveryKeyStored bool                 `json:"-"`
}

type UpdateSystemBackupPolicies struct {
	Policies []UpdateSystemBackupPolicy `json:"policies"`
}

type SystemBackupStorage struct {
	UsedBytes int64 `json:"usedBytes" minimum:"0"`
	FreeBytes int64 `json:"freeBytes" minimum:"0"`
}

type SystemBackupMaintenance struct {
	Active     bool   `json:"active"`
	ReasonCode string `json:"reasonCode,omitempty"`
	ActivityID string `json:"activityId,omitempty"`
}

type SystemBackupOperation struct {
	BackupID   string `json:"backupId"`
	ActivityID string `json:"activityId"`
	Status     string `json:"status" enum:"running,success,failed,cancelled"`
}

type CreateSystemBackupRequest struct {
	PolicyID string `json:"policyId,omitempty" pattern:"^[A-Za-z0-9_-]{1,128}$"`
	TargetID string `json:"targetId,omitempty" pattern:"^[A-Za-z0-9_-]{1,128}$"`
	// Legacy engine inputs are deliberately absent from JSON.
	Destination SystemBackupDestination `json:"-"`
	RecoveryKey string                  `json:"-"`
}

// UploadSystemBackupRequest consumes a completed bounded upload session.
type UploadSystemBackupRequest struct {
	UploadID string `json:"uploadId" pattern:"^[A-Za-z0-9_-]{1,128}$"`
}

type RestoreSystemBackupRequest struct {
	ConfirmBackupID string `json:"confirmBackupId" pattern:"^[A-Za-z0-9_-]{1,128}$"`
	RecoveryKey     string `json:"-"`
}

type DeleteSystemBackupRequest struct {
	RecoveryKey string `json:"-"`
}

// These Go-only values keep the volume recovery-key CLI compiling during the cutover.
type SystemBackupRecoveryKeyStatus struct {
	Configured bool `json:"-"`
}
type SystemBackupRecoveryKey struct {
	RecoveryKey string `json:"-"`
}
type RestoreSystemBackupFilesRequest struct {
	RestoreSelection
	RecoveryKey string `json:"-"`
}
