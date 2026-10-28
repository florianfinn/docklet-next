// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original volume backup lifecycle and target foundation.

package restore

import "time"

// RestoreRun keeps this run's stopped set separate from inherited pending rows.
type Run struct {
	ID                string `gorm:"primaryKey;not null"`
	VolumeName        string `gorm:"not null;index:idx_volume_restore_runs_volume_started,priority:1"`
	Kind              string `gorm:"not null"`
	BackupID          string
	SafetyBackupID    string
	InputPath         string
	InputSHA256       string `gorm:"column:input_sha256"`
	Phase             string `gorm:"not null"`
	Status            string `gorm:"not null"`
	PendingRestart    bool   `gorm:"not null;default:false"`
	StoppedContainers *string
	ErrorCode         string
	ActivityID        string
	StartedAt         time.Time `gorm:"not null;index:idx_volume_restore_runs_volume_started,priority:2"`
	FinishedAt        *time.Time
	Mode              string `gorm:"not null;default:replace"`
	NewVolumeName     string
	CreatedVolume     bool `gorm:"not null;default:false"`
	CleanupPending    bool `gorm:"not null;default:false"`
}

func (*Run) TableName() string { return "volume_restore_runs" }
