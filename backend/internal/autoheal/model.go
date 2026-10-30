// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package autoheal

import "time"

type Restart struct {
	ID                   uint   `gorm:"primaryKey"`
	RunID                string `gorm:"uniqueIndex:idx_auto_heal_run_container"`
	ContainerID          string `gorm:"uniqueIndex:idx_auto_heal_run_container;index:idx_auto_heal_container_started"`
	ContainerName        string
	Stack                string
	StartedAt            time.Time `gorm:"index:idx_auto_heal_container_started"`
	BaselineStartedAt    string
	BaselineRestartCount int
	Outcome              string
	SettledAt            *time.Time
	ProjectedAt          *time.Time
}

func (Restart) TableName() string { return "auto_heal_restarts" }
