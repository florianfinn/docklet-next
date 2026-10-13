// Source: getarcaneapp/arcane/types/backup/system_backup.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: functional cut: removed S3 backup destinations.

package backup

import "time"

type SystemBackupDestination string

const (
	SystemBackupDestinationLocal SystemBackupDestination = "local"
)

// System backup run statuses and triggers.
const (
	SystemBackupStatusRunning   = "running"
	SystemBackupStatusSucceeded = "succeeded"
	SystemBackupStatusFailed    = "failed"

	SystemBackupTriggerManual    = "manual"
	SystemBackupTriggerScheduled = "scheduled"
	SystemBackupTriggerSafety    = "safety"
)

type SystemBackupRun struct {
	ActivityID      string                  `json:"activityId,omitempty"`
	ID              string                  `json:"id"`
	Size            int64                   `json:"size"`
	CreatedAt       time.Time               `json:"createdAt"`
	Status          string                  `json:"status"`
	Trigger         string                  `json:"trigger"`
	Destination     SystemBackupDestination `json:"destination"`
	LocalSnapshotID string                  `json:"localSnapshotId,omitempty"`
	PolicyID        string                  `json:"policyId,omitempty"`
	Error           string                  `json:"error,omitempty"`
}

type SystemBackupPolicy struct {
	ID             string           `json:"id"`
	Enabled        bool             `json:"enabled"`
	Schedule       string           `json:"schedule"`
	RetentionCount int              `json:"retentionCount"`
	LocalEnabled   bool             `json:"localEnabled"`
	LastRun        *SystemBackupRun `json:"lastRun,omitempty"`
}

// UpdateBackupPolicy is the shared policy-update payload for every backup
// domain; StopContainers only applies to volume backups.
type UpdateBackupPolicy struct {
	ID             string `json:"id,omitempty"`
	Enabled        bool   `json:"enabled"`
	Schedule       string `json:"schedule"`
	RetentionCount int    `json:"retentionCount"`
	StopContainers bool   `json:"stopContainers,omitempty"`
	LocalEnabled   bool   `json:"localEnabled"`
}

type UpdateSystemBackupPolicy = UpdateBackupPolicy

type SystemBackupPolicyCollection struct {
	Policies          []SystemBackupPolicy `json:"policies"`
	RecoveryKeyStored bool                 `json:"recoveryKeyStored"`
}

type UpdateSystemBackupPolicies struct {
	Policies []UpdateSystemBackupPolicy `json:"policies"`
}

type SystemBackupRecoveryKeyStatus struct {
	Configured bool `json:"configured"`
}

type SystemBackupRecoveryKey struct {
	RecoveryKey string `json:"recoveryKey"`
}

type CreateSystemBackupRequest struct {
	Destination SystemBackupDestination `json:"destination,omitempty"`
	RecoveryKey string                  `json:"recoveryKey,omitempty"`
	PolicyID    string                  `json:"policyId,omitempty"`
}

type RestoreSystemBackupRequest struct {
	RecoveryKey string `json:"recoveryKey"`
}

// RestoreSystemBackupFilesRequest selects project files to restore from a system backup.
type RestoreSystemBackupFilesRequest struct {
	RestoreSelection

	RecoveryKey string `json:"recoveryKey"`
}

type DeleteSystemBackupRequest struct {
	RecoveryKey string `json:"recoveryKey,omitempty"`
}
