// Source: getarcaneapp/arcane/backend/internal/system/children/prune/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; preserve scheduled network-prune removal semantics; M16 bind mounts and backup completion.

package prune

import (
	"context"
	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/system"
)

func (s *Service) executePrunePlanInternal(ctx context.Context, plan prunePlanInternal, previous scheduler.Run, result *system.PruneAllResult, recovery bool) error {
	dockerClient, err := s.dockerClient(ctx)
	if err != nil {
		return err
	}
	for _, resource := range plan.Resources {
		if resource.Kind == "build_cache" {
			continue
		}
		target := scheduler.TargetOutcome{ResourceType: resource.Kind, ID: "prune:" + resource.Kind + ":" + resource.ID, Status: scheduler.Running}
		if pruneTargetCompletedInternal(previous, target.ID) {
			continue
		}
		if progressErr := jobcontext.Progress(ctx, target); progressErr != nil {
			return progressErr
		}
		eligible, exists, pruneResourceEligibleErr := s.pruneResourceEligibleInternal(ctx, dockerClient, plan, &resource)
		if pruneResourceEligibleErr != nil {
			return pruneResourceEligibleErr
		}
		switch {
		case !exists:
			target.Status = scheduler.Succeeded
		case !eligible:
			target.Status = scheduler.Skipped
		default:
			pruneResourceEligibleErr = s.removePruneResourceInternal(ctx, dockerClient, resource)
			problem := imageDTO.Neutral(pruneResourceEligibleErr, "image.remove-failed", 502)
			if resource.Kind == "image" && pruneResourceEligibleErr != nil && (problem.Status == 409 || problem.Status == 404) {
				target.Status = scheduler.Skipped
				target.Message = problem.Problem.Code
				result.ImageSkipped = append(result.ImageSkipped, imageDTO.PruneItem{ID: resource.ID, Code: problem.Problem.Code})
			} else if resource.Kind == "volume" && pruneResourceEligibleErr != nil && volume.IsWorkspaceBusy(pruneResourceEligibleErr) {
				target.Status = scheduler.Skipped
				target.Message = "in-use"
				target.Code = "volume.workspace-busy"
			} else if pruneResourceEligibleErr != nil && !errdefs.IsNotFound(pruneResourceEligibleErr) {
				target.Status = scheduler.Failed
				target.Message = pruneResourceEligibleErr.Error()
				result.Success = false
				result.Errors = append(result.Errors, pruneResourceEligibleErr.Error())
			} else {
				target.Status = scheduler.Succeeded
				if pruneResourceEligibleErr != nil {
					// Something else removed it first; do not claim its bytes.
					resource.Size = 0
				}
				if resource.Kind == "image" {
					resource.Size = 0
					result.ImageSpaceReclaimedKnown = new(false)
				}
				recordPrunedResourceInternal(resource, result)
			}
		}
		if targetProgressErr := jobcontext.Progress(ctx, target); targetProgressErr != nil {
			return targetProgressErr
		}
	}
	if len(result.ImagesDeleted) > 0 {
		if deleteRecordsForImagesErr := s.imageUpdateService.DeleteRecordsForImages(ctx, result.ImagesDeleted); deleteRecordsForImagesErr != nil {
			return deleteRecordsForImagesErr
		}
	}
	return s.executePruneCacheInternal(ctx, dockerClient, plan, result, recovery)
}
