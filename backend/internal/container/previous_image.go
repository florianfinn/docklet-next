// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original transactional previous-image slot and conditional ownership; offer compensated failed-update recovery.

package container

import (
	"context"
	"errors"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	dto "github.com/florianfinn/docklet-next/types/container"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *ContainerService) previousImage(ctx context.Context, name string) (*PreviousImage, error) {
	if s.operations == nil || s.operations.db == nil {
		return nil, imageDTO.Failure("update.persistence-failed", 500)
	}
	var slot PreviousImage
	err := s.operations.db.WithContext(ctx).First(&slot, "container_name=?", name).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &slot, err
}
func (s *ContainerService) replacePreviousImage(ctx context.Context, name, id, ref, owner, run string) error {
	if s.operations == nil || s.operations.db == nil {
		return imageDTO.Failure("update.persistence-failed", 500)
	}
	return s.operations.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Slot and pin move together; a failed replacement preserves both old rows.
		if err := tx.Where("reason=? AND owner_id=?", "container-previous", name).Delete(&guard.Protection{}).Error; err != nil {
			return err
		}
		g := &guard.Service{DB: &database.DB{DB: tx}}
		if err := g.Protect(ctx, guard.Protection{ImageID: id, Reason: "container-previous", OwnerID: name}); err != nil {
			return err
		}
		slot := PreviousImage{ContainerName: name, ImageID: id, ImageRef: ref, OwnerContainerID: owner, RunID: run, CreatedAt: time.Now().UTC()}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "container_name"}}, UpdateAll: true}).Create(&slot).Error
	})
}
func (s *ContainerService) advancePreviousImage(ctx context.Context, j *RecreateJournal) error {
	if s.operations == nil || s.operations.db == nil {
		return imageDTO.Failure("update.persistence-failed", 500)
	}
	return s.operations.db.WithContext(ctx).Model(&PreviousImage{}).Where("container_name=? AND run_id=? AND owner_container_id=?", j.OldName, j.RunID, j.OldID).UpdateColumn("owner_container_id", j.NewID).Error
}
func (s *ContainerService) projectPreviousImage(ctx context.Context, details *dto.Details) error {
	if s.operations == nil || s.operations.db == nil {
		return nil
	}
	slot, err := s.previousImage(ctx, details.Name)
	if err != nil {
		return err
	}
	if slot != nil && slot.OwnerContainerID == details.ID {
		details.PreviousImage = &dto.PreviousImage{ImageRef: slot.ImageRef, ImageID: slot.ImageID, CreatedAt: slot.CreatedAt}
	}
	return nil
}
