// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original durable observed-run and failed-candidate recovery storage.

package activity

import "time"

// RuntimeObservedRun lives as long as its receipt, independent of activity history.
// Only canonical admission/acceptance installs the marker; payloads cannot bind it.
type RuntimeObservedRun struct {
	RunID     string    `gorm:"primaryKey;not null"`
	Kind      string    `gorm:"not null"`
	Mode      string    `gorm:"not null"`
	Evidence  string    `gorm:"not null;default:''"`
	Outcome   string    `gorm:"not null;default:''"`
	CreatedAt time.Time `gorm:"not null"`
	SettledAt *time.Time
}

func (RuntimeObservedRun) TableName() string { return "runtime_observed_runs" }
