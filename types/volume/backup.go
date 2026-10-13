// Source: getarcaneapp/arcane/types/volume/backup.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations.

package volume

import (
	"time"

	"github.com/florianfinn/docklet-next/types/backup"
)

type BackupDestination string

const (
	BackupDestinationLocal BackupDestination = "local"

	BackupFormatArchive BackupFormat = "archive"
	BackupFormatRustic  BackupFormat = "rustic"
)

type CreateBackupRequest struct {
	Destination BackupDestination `json:"destination,omitempty" doc:"Optional destination override for this manual backup"`
	PolicyID    string            `json:"policyId,omitempty" doc:"Optional backup policy whose settings should be used"`
}

type BackupFormat string

// Backup is one volume backup run.
type Backup struct {
	ID              string                `json:"id"`
	UpdatedAt       *time.Time            `json:"updatedAt,omitempty"`
	VolumeName      string                `json:"volumeName"`
	Size            int64                 `json:"size"`
	CreatedAt       time.Time             `json:"createdAt"`
	Status          string                `json:"status"`
	Trigger         string                `json:"trigger"`
	Destination     BackupDestination     `json:"destination"`
	Format          BackupFormat          `json:"format"`
	LocalSnapshotID string                `json:"localSnapshotId,omitempty"`
	PolicyID        string                `json:"policyId,omitempty"`
	Error           string                `json:"error,omitempty"`
	ActivityID      *string               `json:"activityId,omitempty"`
	Type            backup.ManagementType `json:"type"`
}

type BackupEntry struct {
	ActivityID      *string               `json:"activityId,omitempty"`
	ID              string                `json:"id" doc:"Unique identifier of the backup"`
	VolumeName      string                `json:"volumeName" doc:"Name of the volume"`
	Size            int64                 `json:"size" doc:"Total size of the backup contents"`
	CreatedAt       string                `json:"createdAt" doc:"When the backup was created"`
	Status          string                `json:"status" doc:"Backup result status"`
	Trigger         string                `json:"trigger" doc:"How the backup was started"`
	Destination     BackupDestination     `json:"destination" doc:"Requested backup storage target"`
	Format          BackupFormat          `json:"format" doc:"Storage format of the backup: legacy tar.gz archive or Rustic snapshot"`
	LocalSnapshotID string                `json:"localSnapshotId,omitempty" doc:"Snapshot ID in the local Rustic repository"`
	PolicyID        string                `json:"policyId,omitempty" doc:"Backup policy that created the backup"`
	Error           string                `json:"error,omitempty" doc:"Backup error when the run failed"`
	Type            backup.ManagementType `json:"type" doc:"Whether the backup was system-managed or volume-managed"`
}

type BackupPolicy struct {
	ID             string       `json:"id"`
	VolumeName     string       `json:"volumeName"`
	Enabled        bool         `json:"enabled"`
	Schedule       string       `json:"schedule"`
	RetentionCount int          `json:"retentionCount"`
	StopContainers bool         `json:"stopContainers"`
	LocalEnabled   bool         `json:"localEnabled"`
	LastRun        *BackupEntry `json:"lastRun,omitempty"`
}

type UpdateBackupPolicy = backup.UpdateBackupPolicy

type BackupPolicyCollection struct {
	Policies []BackupPolicy `json:"policies"`
}

type UpdateBackupPolicies struct {
	Policies []UpdateBackupPolicy `json:"policies"`
}
