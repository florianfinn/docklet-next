// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Restore-point discovery and explicit pin release, including orphaned stacks.

package project

import (
	"context"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"maps"
	"slices"
)

func (s *ProjectService) LifecycleRegistration(ctx context.Context) string {
	var binding struct{ RegistrationID string }
	if s.db.WithContext(ctx).Table("runtime_executor_binding").Where("id = 1").Take(&binding).Error == nil {
		return binding.RegistrationID
	}
	return s.settingsService.GetSettingsConfig().InstanceID.Value
}
func (s *ProjectService) ListImageRestorePoints(ctx context.Context, target string, admin bool) ([]dto.RestorePoint, error) {
	_ = s.ReconcileImageLifecycle(ctx)
	var rows []life.Point
	q := s.db.WithContext(ctx).Order("created_at DESC, id DESC")
	if target != "" {
		q = q.Where("target_id = ?", target)
	} else if !admin {
		return nil, lifecycleFailure("update.invalid-target")
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, lifecycleFailure("update.persistence-failed")
	}
	out := []dto.RestorePoint{}
	registration := s.LifecycleRegistration(ctx)
	for _, point := range rows {
		var p Project
		err := s.db.WithContext(ctx).First(&p, "id = ?", point.TargetID).Error
		orphaned := err != nil || targetLifecycleIdentity(&p) != point.TargetIdentity
		if orphaned && !admin {
			return nil, lifecycleFailure("update.invalid-target")
		}
		row := point.DTO(orphaned)
		row.Images = []dto.PointImage{}
		if point.RegistrationID != registration {
			row.Available = false
			row.Code = "restore.unavailable"
		} else {
			_, snap, e := s.lifecycleStore().Point(ctx, point.ID)
			if e != nil || s.verifyLifecyclePoint(ctx, point, snap) != nil {
				row.Available = false
				row.Code = "restore.unavailable"
			}
			if snap != nil {
				for _, key := range slices.Sorted(maps.Keys(snap.Services)) {
					img := snap.Services[key]
					row.Images = append(row.Images, dto.PointImage{Service: key, Reference: img.Reference, ImageID: img.ImageID, Digest: img.Digest, Present: img.Present, Running: img.Running})
				}
			}
		}
		out = append(out, row)
	}
	return out, nil
}
func (s *ProjectService) DeleteImageRestorePoint(ctx context.Context, target, id string, admin bool) error {
	point, _, err := s.lifecycleStore().Point(ctx, id)
	// Deleting an unavailable point does not require decryptable payload or pins.
	if err != nil {
		if e := s.db.WithContext(ctx).First(&point, "id = ?", id).Error; e != nil {
			return lifecycleFailure("restore.not-found")
		}
	}
	if target != "" && point.TargetID != target {
		return lifecycleFailure("restore.not-found")
	}
	var p Project
	orphaned := s.db.WithContext(ctx).First(&p, "id = ?", point.TargetID).Error != nil || targetLifecycleIdentity(&p) != point.TargetIdentity
	if (target == "" || orphaned) && !admin {
		return lifecycleFailure("update.invalid-target")
	}
	release, err := s.imageService.ImageGuard().Acquire(ctx)
	if err != nil {
		return err
	}
	err = s.deleteLifecyclePoint(ctx, id)
	release()
	if err != nil {
		return err
	}
	_ = s.ReconcileImageLifecycle(ctx)
	return nil
}
