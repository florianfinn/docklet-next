// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 60d2471b62b12e1af64c54060a50102a2e39d1c1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original stateless prune contract implementation for issue #179.
package volume

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker"
)

const anonymousVolumeLabel = "com.docker.volume.anonymous"

func pruneEligible(v volume.Volume) bool {
	return v.Driver == "local" && len(v.Options) == 0 && v.Labels[libarcane.InternalResourceLabel] != "true" && strings.TrimSpace(v.CreatedAt) != ""
}

// volumeReferences always queries all containers, bypassing inventory caches.
func volumeReferences(ctx context.Context, cli *client.Client) (map[string]bool, error) {
	containers, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	refs := make(map[string]bool)
	for _, c := range containers.Items {
		for _, m := range c.Mounts {
			if m.Type == "volume" {
				refs[m.Name] = true
			}
		}
	}
	return refs, nil
}

func (s *VolumeService) PlanVolumePrune(ctx context.Context, all bool) (*volumetypes.PrunePlan, error) {
	cli, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	listed, err := cli.VolumeList(ctx, client.VolumeListOptions{})
	if err != nil {
		return nil, err
	}
	refs, err := volumeReferences(ctx, cli)
	if err != nil {
		return nil, err
	}
	plan := &volumetypes.PrunePlan{Candidates: []volumetypes.PrunePlanCandidate{}}
	for _, v := range listed.Items {
		_, anonymous := v.Labels[anonymousVolumeLabel]
		if !pruneEligible(v) || refs[v.Name] || (!all && !anonymous) {
			continue
		}
		reason := "unused"
		if anonymous {
			reason = "unused-anonymous"
		}
		candidate := volumetypes.PrunePlanCandidate{Name: v.Name, Driver: v.Driver, CreatedAt: v.CreatedAt, Reason: reason}
		plan.Candidates = append(plan.Candidates, candidate)
	}
	slices.SortFunc(plan.Candidates, func(a, b volumetypes.PrunePlanCandidate) int { return strings.Compare(a.Name, b.Name) })
	return plan, nil
}

func (s *VolumeService) ExecuteVolumePrune(ctx context.Context, candidates []volumetypes.PruneCandidate) (*volumetypes.PruneResult, error) {
	return s.executeVolumePrune(ctx, candidates, false)
}

func (s *VolumeService) executeVolumePrune(ctx context.Context, candidates []volumetypes.PruneCandidate, all bool) (*volumetypes.PruneResult, error) {
	result := &volumetypes.PruneResult{Removed: []string{}, Skipped: []volumetypes.PruneSkipped{}, Failed: []volumetypes.PruneFailed{}}
	if len(candidates) == 0 {
		return result, nil
	}
	cli, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	// Docker VolumeList/Inspect omit UsageData. Prune deliberately avoids
	// host-wide DiskUsage scans, so advisory reclaimed bytes remain unknown (0).
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if seen[candidate.Name] {
			continue
		}
		seen[candidate.Name] = true
		s.executePruneCandidate(ctx, cli, candidate, result)
	}
	if len(result.Removed) > 0 {
		docker.InvalidateVolumeUsageCache(cli)
		if s.eventService != nil {
			metadata := buildVolumePruneMetadataInternal(all, len(result.Removed), result.SpaceReclaimed)
			if logErr := s.eventService.LogVolumeEvent(ctx, event.EventTypeVolumePrune, "", "bulk_prune", usertypes.SystemUser.ID, usertypes.SystemUser.Username, "0", metadata); logErr != nil {
				slog.WarnContext(ctx, "could not log volume prune", "error", logErr)
			}
		}
	}
	return result, nil
}

func (s *VolumeService) executePruneCandidate(ctx context.Context, cli *client.Client, candidate volumetypes.PruneCandidate, result *volumetypes.PruneResult) {
	defer s.workspaceLocks.Lock(candidate.Name)()
	skip := func(reason string) {
		result.Skipped = append(result.Skipped, volumetypes.PruneSkipped{Name: candidate.Name, Reason: reason})
	}
	fail := func(code string) {
		result.Failed = append(result.Failed, volumetypes.PruneFailed{Name: candidate.Name, Code: code})
	}
	inspected, err := cli.VolumeInspect(ctx, candidate.Name, client.VolumeInspectOptions{})
	if errdefs.IsNotFound(err) {
		skip("not-found")
		return
	}
	if err != nil {
		fail("volume.inspect-failed")
		return
	}
	if candidate.CreatedAt == "" || inspected.Volume.CreatedAt != candidate.CreatedAt {
		skip("changed")
		return
	}
	if !pruneEligible(inspected.Volume) {
		fail("volume.protected")
		return
	}
	refs, err := volumeReferences(ctx, cli)
	if err != nil {
		fail("volume.usage-failed")
		return
	}
	if refs[candidate.Name] {
		skip("in-use")
		return
	}
	// Non-forced removal closes the usage race with Docker's own reference check.
	_, err = cli.VolumeRemove(ctx, candidate.Name, client.VolumeRemoveOptions{Force: false})
	switch {
	case errdefs.IsNotFound(err):
		skip("not-found")
	case errdefs.IsConflict(err):
		skip("in-use")
	case err != nil:
		fail("volume.remove-failed")
	default:
		result.Removed = append(result.Removed, candidate.Name)
		s.removeHelperEntry(candidate.Name)
		if s.backup != nil {
			s.backup.RemovePolicies(ctx, candidate.Name)
		}
	}
}
