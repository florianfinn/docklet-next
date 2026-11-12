// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original owning-host stop-intent persistence.

package runtimeintent

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"gorm.io/gorm/clause"
	"time"
)

type Row struct {
	ContainerID   string `gorm:"primaryKey"`
	ContainerName string
	ProjectID     string
	RecordedAt    time.Time
}

func (Row) TableName() string { return "runtime_stop_intents" }

func Record(ctx context.Context, db *database.DB, rows []Row, now time.Time) error {
	if len(rows) == 0 {
		return nil
	}
	durable, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	for i := range rows {
		rows[i].RecordedAt = now
	}
	return db.WithContext(durable).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "container_id"}}, DoUpdates: clause.AssignmentColumns([]string{"container_name", "project_id", "recorded_at"})}).Create(&rows).Error
}
