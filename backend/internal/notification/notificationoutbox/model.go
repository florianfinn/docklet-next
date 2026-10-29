// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package notificationoutbox

import (
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
	"uuid"
)

type Row struct {
	ID            string `gorm:"primaryKey"`
	Event         string
	EnvironmentID string
	DedupKey      string        `gorm:"uniqueIndex"`
	Payload       database.JSON `gorm:"type:text"`
	Status        string
	Attempts      int
	LeaseUntil    *time.Time
	CreatedAt     time.Time
	SettledAt     *time.Time
}

func (Row) TableName() string { return "notification_outbox" }

// Insert participates in the producer's transaction; callers own commit and wake.
func Insert(tx *gorm.DB, row Row) error {
	if row.ID == "" {
		row.ID = uuid.New().String()
	}
	if row.Status == "" {
		row.Status = "pending"
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
	}
	result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedup_key"}}, DoNothing: true}).Create(&row)
	if result.Error == nil && result.RowsAffected > 0 {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	return result.Error
}

type Availability struct {
	EnvironmentID     string `gorm:"primaryKey"`
	OfflineSince      time.Time
	NotifiedOfflineAt *time.Time
}

func (Availability) TableName() string { return "host_availability_notifications" }

var wake = make(chan struct{}, 1)

func Wakes() <-chan struct{} { return wake }
