// Source: getarcaneapp/arcane/types/backup/volume_backup.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None; M12 backup foundation.

package backup

import "strings"

// Volume backup run statuses and triggers.
const (
	VolumeBackupStatusPartial     = "partial"
	VolumeBackupStatusUnavailable = "unavailable"
	VolumeBackupStatusRunning     = "running"
	VolumeBackupStatusSucceeded   = "succeeded"
	VolumeBackupStatusFailed      = "failed"

	VolumeBackupTriggerManual    = "manual"
	VolumeBackupTriggerScheduled = "scheduled"
	VolumeBackupTriggerSafety    = "safety"
)

// ManagementTypeForPolicy reports which configuration orchestrated a backup created under policyID.
func ManagementTypeForPolicy(policyID string) ManagementType {
	if strings.HasPrefix(policyID, SystemVolumePolicyPrefix) {
		return ManagementTypeSystem
	}
	return ManagementTypeVolume
}
