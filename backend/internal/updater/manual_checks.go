// Source: docklet-next/issues/595
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original fresh manual checks and host-owned activity projection.

package updater

import (
	"context"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
)

func (s *UpdaterService) manualCheckActivity(ctx context.Context, env, name, kind, action string) func(error) {
	var id string
	if s.deps.Activity != nil {
		row, err := s.deps.Activity.StartActivity(ctx, activity.StartActivityRequest{EnvironmentID: env, Type: activityDTO.TypeImageUpdateCheck, ResourceType: &kind, ResourceName: &name, Metadata: database.JSON{"action": action}, Step: "Checking image updates", LatestMessage: "Update check started"})
		if err == nil {
			id = row.ID
		}
	}
	return func(err error) {
		if id == "" {
			return
		}
		status, message := activityDTO.StatusSuccess, "Update check complete"
		var failure *string
		if err != nil {
			status = activityDTO.StatusFailed
			message = "Update check failed"
			failure = &message
		}
		_, _ = s.deps.Activity.CompleteActivity(context.WithoutCancel(ctx), id, status, message, failure, "Update check complete")
	}
}

func (s *UpdaterService) CheckPolicyProject(ctx context.Context, env, target string) (result *projectDTO.UpdateInfo, err error) {
	if s.config != nil && (s.config.AgentMode || s.config.EdgeAgent) {
		return s.CheckProjectUpdates(ctx, target)
	}
	name := target
	if p, e := s.hubPolicies(); e == nil && p.ports.targets != nil {
		if rows, e := p.ports.targets(ctx, env); e == nil {
			for _, row := range rows {
				if row.Kind == "stack" && row.ID == target {
					name = row.Name
					break
				}
			}
		}
	}
	complete := s.manualCheckActivity(ctx, env, name, "project", "check_project_update")
	defer func() { complete(err) }()
	return s.checkPolicyProject(ctx, env, target)
}

// CheckPolicyContainer resolves the current container instance before checking;
// a saved policy follows its stable name, while the request names an instance.
func (s *UpdaterService) CheckPolicyContainer(ctx context.Context, env, container string) (result *imageDTO.UpdateInfo, err error) {
	p, err := s.hubPolicies()
	if err != nil {
		return nil, err
	}
	rows, err := p.ports.targets(ctx, env)
	if err != nil {
		return nil, err
	}
	var target updateTarget
	for _, row := range rows {
		if row.Kind == "container" && row.ContainerID == container {
			target = row
			break
		}
	}
	if target.ContainerID == "" {
		return nil, imageDTO.Failure("update.target-not-found", 404)
	}
	if !target.Eligible {
		return nil, imageDTO.Failure("container.origin-locked", 409)
	}
	complete := s.manualCheckActivity(ctx, env, target.Name, "container", "check_container_update")
	defer func() { complete(err) }()
	reg, err := p.ports.host(ctx, env)
	if err != nil {
		return nil, err
	}
	one, fenceErr := p.store.Policy(ctx, env, reg, target.ID, "container")
	if fenceErr == nil && one.Revision == 0 {
		one.TargetIdentity = target.Name
		one, fenceErr = p.store.EnsureObservedPolicy(ctx, one)
	}
	check, err := p.ports.checkContainer(ctx, env, target)
	if err != nil {
		return nil, err
	}
	if check == nil || check.Status == "failed" || check.Status == "unknown" || check.Status == "error" || check.Status == "not_pulled" {
		return nil, imageDTO.Failure("update.check-unavailable", 503)
	}
	if fenceErr == nil {
		fenceErr = s.bookkeepManualPolicyRow(ctx, p, one, check)
	}
	// Candidate bookkeeping has its own status on the policy; the observation remains usable.
	row, ok := check.ServiceUpdates[target.Name]
	if !ok || row.UpdateInfo == nil {
		return nil, imageDTO.Failure("update.check-unavailable", 503)
	}
	return row.UpdateInfo, nil
}

func (h *UpdaterHandler) CheckContainerUpdate(ctx context.Context, in *dto.CheckContainerInput) (*handlerutil.Out[*imageDTO.UpdateInfo], error) {
	ps, _ := middleware.PermissionsFromContext(ctx)
	if ps == nil || !ps.IsGlobalAdmin() || !ps.Allows(authz.PermImageUpdatesCheck, in.EnvironmentID) {
		return nil, imageDTO.Failure("update.permission-denied", 403)
	}
	if h.updaterService == nil {
		return nil, imageDTO.Failure("update.check-unavailable", 503)
	}
	info, err := h.updaterService.CheckPolicyContainer(ctx, in.EnvironmentID, in.ContainerID)
	if err != nil {
		return nil, err
	}
	return &handlerutil.Out[*imageDTO.UpdateInfo]{Body: base.ApiResponse[*imageDTO.UpdateInfo]{Success: true, Data: info}}, nil
}
