// Source: getarcaneapp/arcane/types/backup/system_volume_backup.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: functional cut: removed S3 backup destinations.; Retain historical management classification and shared archive/history DTOs; remove centralized volume policy and selection vocabulary.

package backup

import "time"

// ManagementType identifies the historical owner of a retained backup row.
type ManagementType string

const (
	ManagementTypeSystem ManagementType = "system"
	ManagementTypeVolume ManagementType = "volume"
	// SystemVolumePolicyPrefix classifies history from the removed centralized owner.
	SystemVolumePolicyPrefix = "system-volume:"
)

// HistoryEntry is a common view over Arcane recovery and volume backup records.
type HistoryEntry struct {
	ID              string                  `json:"id" sortable:"true"`
	Size            int64                   `json:"size" sortable:"true"`
	CreatedAt       time.Time               `json:"createdAt" sortable:"true"`
	Status          string                  `json:"status" sortable:"true"`
	Trigger         string                  `json:"trigger" sortable:"true"`
	Destination     SystemBackupDestination `json:"destination" sortable:"true"`
	Format          string                  `json:"format,omitempty"`
	LocalSnapshotID string                  `json:"localSnapshotId,omitempty"`
	PolicyID        string                  `json:"policyId,omitempty"`
	Error           string                  `json:"error,omitempty"`
	Type            ManagementType          `json:"type" sortable:"true"`
	ResourceType    string                  `json:"resourceType"`
	ResourceName    string                  `json:"resourceName" sortable:"true"`
}

// BackupRunAccepted identifies a backup operation accepted for background execution.
type BackupRunAccepted struct {
	ActivityID string `json:"activityId"`
	Status     string `json:"status"`
}
