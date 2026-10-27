// Source: getarcaneapp/arcane/backend/internal/system/children/prune/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; preserve scheduled network-prune removal semantics.

package prune

import (
	"context"
	"encoding/json/v2"

	"slices"

	"strings"

	"github.com/containerd/errdefs"

	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/system"

	"github.com/moby/moby/client"

	"go.getarcane.app/docker/compat"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"

	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
)

// pruneResourceEligibleInternal re-checks a planned resource against live state and refreshes its size from that snapshot.
func (s *Service) pruneResourceEligibleInternal(ctx context.Context, dockerClient *client.Client, plan prunePlanInternal, resource *pruneResourceInternal) (eligible, exists bool, err error) {
	if resource.Kind == "network" {
		inspected, networkInspectWithCompatibilityErr := compat.NetworkInspectWithCompatibility(ctx, dockerClient, resource.ID, client.NetworkInspectOptions{})
		if errdefs.IsNotFound(networkInspectWithCompatibilityErr) {
			return false, false, nil
		}
		if networkInspectWithCompatibilityErr != nil {
			return false, false, networkInspectWithCompatibilityErr
		}
		return len(inspected.Network.Containers) == 0 && !inspected.Network.Ingress, true, nil
	}
	usage, err := dockerClient.DiskUsage(ctx, client.DiskUsageOptions{Containers: resource.Kind == "container", Images: resource.Kind == "image", Volumes: resource.Kind == "volume", Verbose: true})
	if err != nil {
		return false, false, err
	}
	switch resource.Kind {
	case "container":
		for _, item := range usage.Containers.Items {
			if item.ID == resource.ID {
				resource.Size = item.SizeRw
				return item.State != "running" && item.State != "paused" && item.State != "restarting", true, nil
			}
		}
	case "image":
		return pruneImageEligibleInternal(usage, plan.Request.Images, resource)
	case "volume":
		for _, item := range usage.Volumes.Items {
			if item.Name == resource.ID {
				if item.UsageData == nil {
					return false, true, nil
				}
				resource.Size = item.UsageData.Size
				return item.UsageData.RefCount == 0 && !strings.EqualFold(item.Labels[libarcane.InternalResourceLabel], "true"), true, nil
			}
		}
	}
	return false, false, nil
}

func pruneImageEligibleInternal(usage client.DiskUsageResult, request *system.PruneImagesOptions, resource *pruneResourceInternal) (eligible, exists bool, err error) {
	for _, item := range usage.Images.Items {
		if item.ID == resource.ID {
			// Size - SharedSize is the unique layer size Docker counts as reclaimable for an unused image.
			resource.Size = 0
			if item.SharedSize >= 0 {
				resource.Size = item.Size - item.SharedSize
			}
			for _, tag := range item.RepoTags {
				if !slices.Contains(resource.Names, tag) {
					return false, true, nil
				}
			}
			return item.Containers == 0 && (request.Mode != system.PruneImageModeDangling || len(item.RepoTags) == 0), true, nil
		}
	}
	return false, false, nil
}

func (s *Service) ReconcileScheduledPrune(ctx context.Context, previous scheduler.Run) (scheduler.Outcome, error) {
	outcome := jobcontext.ConfirmedTarget(previous, "scheduled-prune")
	if outcome.Status == scheduler.Succeeded {
		return outcome, nil
	}
	for _, target := range previous.Outcome.Targets {
		if target.ID != "prune-plan" || len(target.RecoveryData) == 0 {
			continue
		}
		var plan prunePlanInternal
		if err := json.Unmarshal(target.RecoveryData, &plan); err != nil {
			return outcome, err
		}
		prune := s.beginSystemPruneInternal(ctx, previous.EnvironmentID, plan.Request)
		if prune.started.IsAbsent() {
			return outcome, nil
		}
		defer s.finishSystemPruneInternal(previous.EnvironmentID) //nolint:gocritic // The matching recovery target always returns before the loop advances.
		result := &system.PruneAllResult{Success: true}
		err := s.executePrunePlanInternal(ctx, plan, previous, result, true)
		if err != nil {
			result.Success = false
			result.Errors = append(result.Errors, err.Error())
		}
		s.completeSystemPruneActivityInternal(ctx, prune.activityID, result)
		if err != nil {
			outcome.Message = err.Error()
			return outcome, nil
		}
		if !result.Success {
			return scheduler.Outcome{Status: scheduler.Partial, Targets: previous.Outcome.Targets}, nil
		}
		if progressErr := jobcontext.Progress(ctx, scheduler.TargetOutcome{ID: "scheduled-prune", Status: scheduler.Succeeded}); progressErr != nil {
			return outcome, progressErr
		}
		return scheduler.Outcome{Status: scheduler.Succeeded}, nil
	}
	return outcome, nil
}

func pruneTargetCompletedInternal(previous scheduler.Run, targetID string) bool {
	for _, target := range previous.Outcome.Targets {
		if target.ID == targetID && (target.Status == scheduler.Succeeded || target.Status == scheduler.Skipped) {
			return true
		}
	}
	return false
}
