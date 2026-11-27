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

// pruneResourceEligibleInternal re-checks a planned resource against live state without repeating size calculations.
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
	switch resource.Kind {
	case "container":
		item, err := compat.ContainerInspectWithCompatibility(ctx, dockerClient, resource.ID, client.ContainerInspectOptions{})
		if errdefs.IsNotFound(err) {
			return false, false, nil
		}
		if err != nil {
			return false, false, err
		}
		state := item.Container.State
		return state != nil && !state.Running && !state.Paused && !state.Restarting, true, nil
	case "image":
		item, err := dockerClient.ImageInspect(ctx, resource.ID)
		if errdefs.IsNotFound(err) {
			return false, false, nil
		}
		if err != nil {
			return false, false, err
		}
		for _, tag := range item.RepoTags {
			if !slices.Contains(resource.Names, tag) {
				return false, true, nil
			}
		}
		listed, err := dockerClient.ContainerList(ctx, client.ContainerListOptions{All: true})
		if err != nil {
			return false, false, err
		}
		for _, c := range listed.Items {
			if c.ImageID == resource.ID {
				return false, true, nil
			}
		}
		return plan.Request.Images.Mode != system.PruneImageModeDangling || len(item.RepoTags) == 0, true, nil
	case "volume":
		item, err := dockerClient.VolumeInspect(ctx, resource.ID, client.VolumeInspectOptions{})
		if errdefs.IsNotFound(err) {
			return false, false, nil
		}
		if err != nil {
			return false, false, err
		}
		if strings.EqualFold(item.Volume.Labels[libarcane.InternalResourceLabel], "true") {
			return false, true, nil
		}
		listed, err := dockerClient.ContainerList(ctx, client.ContainerListOptions{All: true})
		if err != nil {
			return false, false, err
		}
		for _, c := range listed.Items {
			for _, m := range c.Mounts {
				if m.Type == "volume" && m.Name == resource.ID {
					return false, true, nil
				}
			}
		}
		return true, true, nil
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
