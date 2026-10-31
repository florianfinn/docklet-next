// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M14 container and host operations foundation.

package container

import (
	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	"time"
)

// RecreateJournal records intent and observed identity around every Docker effect.
type RecreateJournal struct {
	RunID      string    `gorm:"primaryKey;not null"`
	OldID      string    `gorm:"not null"`
	OldName    string    `gorm:"not null"`
	BackupName string    `gorm:"not null"`
	NewID      string    `gorm:"not null"`
	NewName    string    `gorm:"not null"`
	WasRunning bool      `gorm:"not null"`
	AutoRemove bool      `gorm:"not null"`
	Phase      string    `gorm:"not null"`
	UpdatedAt  time.Time `gorm:"not null"`
}

func (RecreateJournal) TableName() string { return "container_recreate_journal" }

// OriginOverride is shared with hub projection without a module dependency cycle.
type OriginOverride = origin.Override
