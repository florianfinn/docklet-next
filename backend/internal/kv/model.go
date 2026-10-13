// Source: getarcaneapp/arcane/backend/internal/kv/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package kv

import "time"

// KVEntry stores lightweight application state as arbitrary key/value pairs.
type KVEntry struct {
	Key       string     `gorm:"column:key;primaryKey"`
	Value     string     `gorm:"column:value"`
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt *time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (KVEntry) TableName() string {
	return "kv"
}
