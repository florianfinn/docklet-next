// Source: getarcaneapp/arcane/backend/internal/event/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; count-based event retention.

package event

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"time"
)

func (s *EventService) DeleteOldEvents(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	var deleted int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("timestamp < ?", cutoff).Delete(&Event{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete old events: %w", result.Error)
		}
		deleted = result.RowsAffected
		return nil
	})
	if err == nil && deleted > 0 {
		s.changes.Publish(struct{}{})
	}
	return err
}

// PruneEvents keeps the newest maxEntries events by occurrence time.
func (s *EventService) PruneEvents(ctx context.Context, maxEntries int) (int64, error) {
	if maxEntries < 0 {
		return 0, errors.New("event limit must not be negative")
	}
	result := s.db.WithContext(ctx).Exec(`DELETE FROM events WHERE id IN (
  SELECT id FROM events ORDER BY timestamp DESC, id DESC OFFSET ?
 )`, maxEntries)
	if result.Error != nil {
		return 0, fmt.Errorf("prune events: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		s.changes.Publish(struct{}{})
	}
	return result.RowsAffected, nil
}
