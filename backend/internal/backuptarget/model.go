// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original volume backup lifecycle and target foundation.

package backuptarget

import "time"

// Target persists secrets only as independently encrypted values on each host.
type Target struct {
	ID             string `gorm:"primaryKey;not null"`
	Name           string `gorm:"not null"`
	Kind           string `gorm:"not null"`
	HostID         *string
	Path           string    `gorm:"not null"`
	Server         string    `gorm:"not null"`
	Share          string    `gorm:"not null"`
	Username       string    `gorm:"not null"`
	SecretEnc      string    `gorm:"not null"`
	Options        string    `gorm:"not null"`
	RecoveryKeyEnc string    `gorm:"not null"`
	ConfigVersion  int64     `gorm:"not null"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      *time.Time
	RetiredAt      *time.Time
}

func (*Target) TableName() string { return "backup_targets" }

// Check records the configuration admitted for testing, rather than completion-time settings.
type Check struct {
	TargetID      string `gorm:"primaryKey;not null"`
	HostID        string `gorm:"primaryKey;not null"`
	SetVersion    int64  `gorm:"not null"`
	ConfigVersion int64  `gorm:"not null"`
	ActivityID    string `gorm:"not null"`
	OK            bool   `gorm:"not null"`
	Code          string
	FreeBytes     *int64
	TotalBytes    *int64
	SnapshotCount *int64
	At            time.Time `gorm:"not null"`
}

func (*Check) TableName() string { return "backup_target_checks" }
