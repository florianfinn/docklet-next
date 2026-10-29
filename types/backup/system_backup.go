// Source: getarcaneapp/arcane/types/backup/system_backup.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Replaced the system recovery wire contract with coordinated SQLite archives; M12 backup foundation.

package backup

import (
	"errors"
	"time"
)

// SystemBackupDestination remains the volume-backup destination vocabulary.
type SystemBackupDestination string

const SystemBackupDestinationLocal SystemBackupDestination = "local"

const MaxSystemArchiveBytes int64 = 2 << 30

const (
	SystemBackupErrorInterrupted = "backup.interrupted"
	SystemBackupErrorFailed      = "backup.failed"
	SystemBackupToolsUnavailable = "backup.tools_unavailable"
	SystemBackupStorageMissing   = "backup.storage_unavailable"
)

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
	return id + ".tar", nil
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

// SystemBackupRun describes one versioned two-file SQLite archive.
type SystemBackupRun struct {
	ID               string                `json:"id"`
	ActivityID       string                `json:"activityId,omitempty"`
	ArchiveName      string                `json:"archiveName,omitempty"`
	Size             int64                 `json:"size" minimum:"0" maximum:"2147483648" doc:"Archive size in bytes"`
	CreatedAt        time.Time             `json:"createdAt"`
	CompletedAt      *time.Time            `json:"completedAt,omitempty"`
	Status           string                `json:"status" enum:"running,succeeded,failed"`
	Trigger          string                `json:"trigger" enum:"manual,scheduled,uploaded"`
	PolicyID         string                `json:"policyId,omitempty"`
	ArchiveVersion   int                   `json:"archiveVersion,omitempty" minimum:"0"`
	MigrationVersion int64                 `json:"migrationVersion,omitempty" minimum:"0"`
	CopyTargetID     string                `json:"copyTargetId,omitempty"`
	CopyState        SystemBackupCopyState `json:"copyState" enum:"none,pending,succeeded,failed"`
	CopyErrorCode    string                `json:"copyErrorCode,omitempty"`
	ErrorCode        string                `json:"errorCode,omitempty"`

	// System-managed volume policies still map their snapshot records through these Go-only fields.
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
}

// UpdateBackupPolicy is shared with system-managed volume backup policies.
type UpdateBackupPolicy struct {
	ID             string `json:"id,omitempty"`
	Enabled        bool   `json:"enabled"`
	Schedule       string `json:"schedule"`
	RetentionCount int    `json:"retentionCount" minimum:"0" maximum:"3650"`
	TargetID       string `json:"targetId,omitempty"`
	StopContainers bool   `json:"stopContainers,omitempty"`
	LocalEnabled   bool   `json:"localEnabled"`
}

type UpdateSystemBackupPolicy struct {
	ID             string `json:"id,omitempty" pattern:"^[A-Za-z0-9_-]{1,128}$"`
	Enabled        bool   `json:"enabled"`
	Schedule       string `json:"schedule"`
	RetentionCount int    `json:"retentionCount" minimum:"0" maximum:"3650"`
	TargetID       string `json:"targetId,omitempty" pattern:"^[A-Za-z0-9_-]{1,128}$"`
}

type SystemBackupPolicyCollection struct {
	Policies []SystemBackupPolicy `json:"policies"`
}

type UpdateSystemBackupPolicies struct {
	Policies []UpdateSystemBackupPolicy `json:"policies"`
}

type SystemBackupStorage struct {
	Available       bool   `json:"available"`
	ReasonCode      string `json:"reasonCode,omitempty" enum:"backup.tools_unavailable,backup.storage_unavailable"`
	UsedBytes       int64  `json:"usedBytes" minimum:"0"`
	FreeBytes       int64  `json:"freeBytes" minimum:"0"`
	MaxArchiveBytes int64  `json:"maxArchiveBytes" minimum:"2147483648" maximum:"2147483648"`
}

type SystemBackupMaintenance struct {
	Operation  string `json:"operation,omitempty" enum:"backup,restore"`
	Active     bool   `json:"active"`
	Phase      string `json:"phase,omitempty" enum:"draining,applying,verifying,blocked"`
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
}

type RestoreSystemBackupRequest struct {
	ConfirmBackupID string `json:"confirmBackupId" pattern:"^[A-Za-z0-9_-]{1,128}$"`
	RecoveryKey     string `json:"-"`
}

// Volume recovery-key operations use these Go-only values.
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
