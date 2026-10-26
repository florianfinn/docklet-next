// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Recheck deferred retention decisions against current anchors and holds.

package lifecycle

import (
	"context"
	"slices"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"gorm.io/gorm"
)

// TrimPoint re-plans a candidate in the deletion transaction, since settlement
// or restore selection may have changed its anchor or hold after the first plan.
func (s Store) TrimPoint(ctx context.Context, target, id string) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var points []Point
		if err := tx.Where("target_id = ? AND available = ?", target, true).Order("created_at DESC, id DESC").Find(&points).Error; err != nil {
			return err
		}
		store := Store{DB: &database.DB{DB: tx}}
		held := map[string]bool{}
		for _, point := range points {
			active, err := store.Held(ctx, point.ID)
			if err != nil {
				return err
			}
			held[point.ID] = active
		}
		if !slices.Contains(RetentionDeletions(points, held), id) {
			return nil
		}
		return tx.Delete(&Point{}, "id = ?", id).Error
	})
}
