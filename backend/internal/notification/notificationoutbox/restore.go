// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package notificationoutbox

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"gorm.io/gorm"
	"time"
)

func ExpireRestored(ctx context.Context, db *database.DB) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Row{}).Where("status IN ?", []string{"pending", "sending"}).Updates(map[string]any{"status": "expired", "lease_until": nil, "settled_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		return tx.Where("1=1").Delete(&Availability{}).Error
	})
}
