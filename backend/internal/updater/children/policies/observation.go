// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original conditional observation creation and policy windows.

package policies

import (
	"context"
	"errors"

	"github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (p Policy) Window() *dto.UpdateWindow {
	if p.WindowKind == nil || p.WindowTime == nil {
		return nil
	}
	w := &dto.UpdateWindow{Kind: *p.WindowKind, Time: *p.WindowTime}
	if p.WindowDay != nil {
		w.Day = new(*p.WindowDay)
	}
	return w
}

func (p *Policy) SetWindow(w *dto.UpdateWindow) {
	p.WindowKind, p.WindowTime, p.WindowDay = nil, nil, nil
	if w != nil {
		p.WindowKind, p.WindowTime = new(w.Kind), new(w.Time)
		if w.Day != nil {
			p.WindowDay = new(*w.Day)
		}
	}
}

func environmentExists(tx *gorm.DB, env string) error {
	var count int64
	if err := tx.Table("environments").Where("id = ?", env).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return image.Failure("update.target-changed", 409)
	}
	return nil
}

// EnsureObservedPolicy never recreates a deleted environment or changes a
// concurrently saved policy. The returned revision is the observation's fence.
func (s Store) EnsureObservedPolicy(ctx context.Context, p Policy) (Policy, error) {
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := environmentExists(tx, p.EnvironmentID); err != nil {
			return err
		}
		var current Policy
		err := tx.Where("environment_id = ? AND target_kind = ? AND target_id = ?", p.EnvironmentID, p.TargetKind, p.TargetID).Take(&current).Error
		if err == nil {
			if current.RegistrationID != p.RegistrationID {
				return image.Failure("update.target-changed", 409)
			}
			p = current
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		p.Revision = 1
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&p).Error; err != nil {
			return err
		}
		return tx.First(&p, "id = ?", p.ID).Error
	})
	return p, err
}
