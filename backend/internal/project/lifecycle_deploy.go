// Source: docklet-next/issues/344
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original deploy-point preparation from last recorded baseline evidence.

package project

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// deployPointLeaseWait bounds how long a deploy waits for the host image lease
// before it proceeds without a point.
var deployPointLeaseWait = 10 * time.Second

// acquireDeployPointLease retries the host image try-lease, so short holders
// such as another prepare or a post-settlement reconcile do not cost the point.
func (s *ProjectService) acquireDeployPointLease(ctx context.Context) (func(), bool) {
	images := s.imageService.ImageGuard()
	deadline := time.Now().Add(deployPointLeaseWait)
	for {
		if release, err := images.Acquire(ctx); err == nil {
			return release, true
		}
		// Without admission the guard can never grant the lease; waiting is pointless.
		if images == nil || images.Admission == nil || !time.Now().Before(deadline) {
			return nil, false
		}
		select {
		case <-ctx.Done():
			return nil, false
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// PrepareDeployPoint never refuses a deploy. The baseline payload, including
// workload presence, is historical evidence; current files only check support.
func (s *ProjectService) PrepareDeployPoint(ctx context.Context, id, run, registration string) {
	result := dto.LifecycleResult{RunID: run}
	defer func() { s.recordLifecycleResult(context.WithoutCancel(ctx), run, &result) }()
	unavailable := func(code string) {
		result.RestorePointID, result.PointCode = "", code
		row := life.Run{ID: run, PointCode: &code}
		if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.Assignments(map[string]any{"point_id": "", "point_code": code})}).Create(&row).Error; err != nil {
			result.PointCode = "update.persistence-failed"
		}
	}
	release, ok := s.acquireDeployPointLease(ctx)
	if !ok {
		unavailable("image.busy")
		return
	}
	defer release()
	p, err := s.getMutableProject(ctx, id)
	if err != nil {
		unavailable("deploy.point-unavailable")
		return
	}
	model, _, err := s.loadComposeProjectForProject(ctx, p, nil)
	if err != nil {
		unavailable("deploy.point-unavailable")
		return
	}
	if len(model.ComposeFiles) != 1 {
		unavailable("update.image-binding-unsupported")
		return
	}
	includes, includeErr := projects.ParseIncludes(model.ComposeFiles[0], nil, false)
	if includeErr != nil || len(includes) != 0 {
		unavailable("update.image-binding-unsupported")
		return
	}
	var row life.Baseline
	err = s.db.WithContext(ctx).First(&row, "target_id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		unavailable("deploy.point-unavailable")
		return
	}
	if err != nil {
		unavailable("update.persistence-failed")
		return
	}
	if row.TargetIdentity != targetLifecycleIdentity(p) || row.RegistrationID != registration {
		unavailable("deploy.point-unavailable")
		return
	}
	saved, err := life.Open(row.Payload)
	if err != nil || saved.TargetID != id || saved.TargetIdentity != row.TargetIdentity || saved.RegistrationID != registration {
		unavailable("deploy.point-unavailable")
		return
	}
	if !deployPointSnapshotSupported(saved, p.Path) {
		unavailable("update.image-binding-unsupported")
		return
	}
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		unavailable("deploy.point-unavailable")
		return
	}
	for _, img := range saved.Images() {
		if !img.Present {
			continue
		}
		found, e := c.ImageInspect(ctx, img.ImageID)
		if e != nil || found.ID != img.ImageID {
			unavailable("deploy.point-unavailable")
			return
		}
	}
	point, err := s.lifecycleStore().Prepare(ctx, saved, run, "", "settled")
	if err != nil {
		unavailable("update.persistence-failed")
		return
	}
	if err = s.lifecyclePinPoint(ctx, point, saved); err != nil {
		// Incomplete pins are unavailable; metadata-free protections remain cleanup
		// work until reconciliation can remove every installed retention tag.
		_ = s.db.WithContext(context.WithoutCancel(ctx)).Delete(&life.Point{}, "id = ?", point.ID).Error
		unavailable("update.persistence-failed")
		return
	}
	result.RestorePointID = point.ID
}

// Mark immediately before Compose deploy (also used after redeploy's pull).
// Failure to persist evidence is a point hint, never a deploy refusal.
func (s *ProjectService) markDeployPointEffect(ctx context.Context) {
	execution := dto.ExecutionIdentityFromContext(ctx)
	if execution.RunID == "" {
		return
	}
	var run life.Run
	err := s.db.WithContext(ctx).First(&run, "id = ?", execution.RunID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && run.PointID == "" {
		return
	}
	if err == nil {
		err = s.lifecycleStore().Effect(ctx, execution.RunID)
	}
	if err != nil {
		code := "update.persistence-failed"
		// Without an effect marker the point cannot safely survive restart.
		// Withdraw it before Compose starts, retaining pin cleanup for retry.
		_ = s.db.WithContext(context.WithoutCancel(ctx)).Transaction(func(tx *gorm.DB) error {
			if err := tx.Delete(&life.Point{}, "source_run_id = ?", execution.RunID).Error; err != nil {
				return err
			}
			return tx.Model(&life.Run{}).Where("id = ?", execution.RunID).Updates(map[string]any{"point_id": "", "point_code": code}).Error
		})
		result := dto.LifecycleResult{RunID: execution.RunID, PointCode: code}
		s.recordLifecycleResult(context.WithoutCancel(ctx), execution.RunID, &result)
	}
}

func (s *ProjectService) deployPointResult(ctx context.Context, run string) dto.LifecycleResult {
	result := dto.LifecycleResult{RunID: run}
	var row life.Run
	if s.db.WithContext(ctx).First(&row, "id = ?", run).Error == nil {
		result.RestorePointID = row.PointID
		if row.PointCode != nil {
			result.PointCode = *row.PointCode
		}
	}
	return result
}

// Older baseline capture may have retained only the root file of an include
// stack. Inspect the recorded file too, even if today's files no longer include.
func deployPointSnapshotSupported(snapshot *life.Snapshot, projectPath string) bool {
	if snapshot.Model == nil || len(snapshot.Model.ComposeFiles) != 1 {
		return false
	}
	composePath := snapshot.Model.ComposeFiles[0]
	rel, err := filepath.Rel(projectPath, composePath)
	if err != nil {
		return false
	}
	for _, file := range snapshot.Files {
		if file.Path != filepath.ToSlash(rel) || !file.Present {
			continue
		}
		includes, err := projects.ParseIncludesFromContent(composePath, file.Content, nil, false)
		return err == nil && len(includes) == 0
	}
	return false
}
