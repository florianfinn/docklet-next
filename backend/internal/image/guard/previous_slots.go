// Source: docklet-next/issues/409
// Revision: 45ee2635d68e71671b571cbd8970425ddd194b59
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original stale previous-image slot cleanup before image prune.

package guard

import (
	"context"

	"github.com/moby/moby/client"
	"gorm.io/gorm"
)

// previousSlot reads the container module's slot table; the guard owns only
// the matching container-previous pin.
type previousSlot struct {
	ContainerName    string
	OwnerContainerID string
}

func (previousSlot) TableName() string { return "container_previous_images" }

// releaseStalePreviousSlots drops slots whose owning container no longer exists,
// with their pins, so prune can reclaim the protected image. A container that a
// running recreate renamed keeps its ID and therefore its slot.
func (s *Service) releaseStalePreviousSlots(ctx context.Context, c *client.Client) error {
	if s.DB == nil {
		return nil
	}
	var slots []previousSlot
	if err := s.DB.WithContext(ctx).Find(&slots).Error; err != nil || len(slots) == 0 {
		return err
	}
	listed, err := c.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return err
	}
	present := map[string]bool{}
	for _, item := range listed.Items {
		present[item.ID] = true
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, slot := range slots {
			if present[slot.OwnerContainerID] {
				continue
			}
			deleted := tx.Where("container_name=? AND owner_container_id=?", slot.ContainerName, slot.OwnerContainerID).Delete(&previousSlot{})
			if deleted.Error != nil {
				return deleted.Error
			}
			if deleted.RowsAffected == 0 {
				continue
			}
			if err := tx.Where("reason=? AND owner_id=?", "container-previous", slot.ContainerName).Delete(&Protection{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
