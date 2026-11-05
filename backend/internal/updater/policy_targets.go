// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original complete inventory reads and read-only eligible target projection.

package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	containers "github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/types/base"
	containerDTO "github.com/florianfinn/docklet-next/types/container"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
)

type updateTarget struct {
	Kind, ID, Name, ContainerID, ImageRef, Platform string
	Eligible, AutoExcluded                          bool
}

func (p *imagePolicyService) findContainerTarget(ctx context.Context, env, name string) (updateTarget, error) {
	if p.ports.targets == nil {
		return updateTarget{}, imageDTO.Failure("update.check-unavailable", 503)
	}
	rows, err := p.ports.targets(ctx, env)
	if err != nil {
		return updateTarget{}, err
	}
	for _, row := range rows {
		if row.Kind == "container" && row.ID == name {
			if !row.Eligible {
				return row, imageDTO.Failure("container.origin-locked", 409)
			}
			return row, nil
		}
	}
	return updateTarget{}, imageDTO.Failure("update.target-not-found", 404)
}

func completeInventory[T any](response base.Paginated[T]) bool {
	return response.Success && response.Data != nil && response.Pagination.CurrentPage == 1 && response.Pagination.TotalPages == 1 && response.Pagination.TotalItems == int64(len(response.Data))
}

func (s *UpdaterService) policyTargets(envs *environment.EnvironmentService, service *containers.ContainerService) func(context.Context, string) ([]updateTarget, error) {
	return func(ctx context.Context, env string) ([]updateTarget, error) {
		var stacks base.Paginated[projectDTO.Details]
		var inventory base.Paginated[containerDTO.Summary]
		if env == "0" {
			if service == nil || s.deps.Projects == nil {
				return nil, imageDTO.Failure("update.check-unavailable", 503)
			}
			params := pagination.QueryParams{Params: pagination.Params{Start: 0, Limit: -1}, Filters: map[string]string{"archived": "all"}}
			projects, page, err := s.deps.Projects.ListProjects(ctx, params)
			if err != nil {
				return nil, err
			}
			stacks = base.Paginated[projectDTO.Details]{Success: true, Data: projects, Pagination: base.PaginationResponse{TotalItems: page.TotalItems, CurrentPage: page.CurrentPage, TotalPages: page.TotalPages}}
			rows, err := service.ListContainersPaginated(ctx, pagination.QueryParams{Params: pagination.Params{Start: 0, Limit: -1}}, true, false, true, "")
			if err != nil {
				return nil, err
			}
			inventory = base.Paginated[containerDTO.Summary]{Success: true, Data: rows.Items, Pagination: base.PaginationResponse{TotalItems: rows.Pagination.TotalItems, CurrentPage: rows.Pagination.CurrentPage, TotalPages: rows.Pagination.TotalPages}}
			if stacks.Data == nil {
				stacks.Data = []projectDTO.Details{}
			}
			if inventory.Data == nil {
				inventory.Data = []containerDTO.Summary{}
			}
		} else {
			if envs == nil {
				return nil, imageDTO.Failure("update.check-unavailable", 503)
			}
			if err := envs.ProxyJSONRequest(ctx, env, http.MethodGet, "/api/environments/0/projects?start=0&limit=-1&archived=all", nil, &stacks); err != nil {
				return nil, err
			}
			if err := envs.ProxyJSONRequest(ctx, env, http.MethodGet, "/api/environments/0/containers?start=0&limit=-1&includeHidden=true", nil, &inventory); err != nil {
				return nil, err
			}
		}
		if !completeInventory(stacks) || !completeInventory(inventory) {
			return nil, imageDTO.Failure("update.check-unavailable", 503)
		}
		if service != nil {
			body, err := json.Marshal(inventory)
			if err != nil {
				return nil, err
			}
			body, err = service.OverlayInventory(ctx, env, body)
			if err != nil {
				return nil, err
			}
			var overlaid base.Paginated[containerDTO.Summary]
			if err := json.Unmarshal(body, &overlaid); err != nil {
				return nil, err
			}
			inventory = overlaid
		}
		return projectUpdateTargets(stacks.Data, inventory.Data)
	}
}

func containerCheckProjection(target updateTarget, response *checkDTO.Response) (*projectDTO.UpdateInfo, error) {
	if response == nil {
		return nil, imageDTO.Failure("update.check-unavailable", 503)
	}
	info := imageCheckInfo(response)
	if one := response.ContainerUpdates[target.ContainerID]; one != nil {
		info = imageCheckInfo(one)
	}
	status := "up_to_date"
	if info.HasUpdate {
		status = "has_update"
	}
	if info.State == "unknown" || info.State == "error" || info.State == "unchecked" {
		status = "unknown"
	}
	return &projectDTO.UpdateInfo{Status: status, HasUpdate: info.HasUpdate, ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{target.Name: {ImageRef: target.ImageRef, UpdateInfo: info}}}, nil
}

func (s *UpdaterService) policyContainerCheck(envs *environment.EnvironmentService) func(context.Context, string, updateTarget) (*projectDTO.UpdateInfo, error) {
	return func(ctx context.Context, env string, target updateTarget) (*projectDTO.UpdateInfo, error) {
		var result *checkDTO.Response
		if env == "0" {
			var err error
			result, err = s.deps.ImageUpdates.ReadUpdateInfo(ctx, target.ImageRef)
			if err != nil {
				return nil, err
			}
		} else {
			var out base.ApiResponse[checkDTO.Response]
			if err := envs.ProxyJSONRequest(ctx, env, http.MethodGet, "/api/environments/0/image-updates/check?imageRef="+url.QueryEscape(target.ImageRef), nil, &out); err != nil {
				return nil, err
			}
			if !out.Success {
				return nil, imageDTO.Failure("update.check-unavailable", 503)
			}
			result = &out.Data
		}
		return containerCheckProjection(target, result)
	}
}

func (s *UpdaterService) ListImagePolicies(ctx context.Context, env string) ([]dto.AutoUpdatePolicy, error) {
	p, err := s.hubPolicies()
	if err != nil {
		return nil, err
	}
	targets, err := p.ports.targets(ctx, env)
	if err != nil {
		return nil, err
	}
	reg, err := p.ports.host(ctx, env)
	if err != nil {
		return nil, err
	}
	present := map[string]bool{}
	for _, target := range targets {
		if target.Eligible {
			present[target.Kind+"\x00"+target.ID] = true
		}
	}
	if err := p.store.PruneAbsent(ctx, env, reg, present, time.Now().UTC()); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []dto.AutoUpdatePolicy{}
	for _, target := range targets {
		if !target.Eligible {
			continue
		}
		row, err := s.GetImagePolicy(ctx, env, target.ID, target.Kind)
		if err != nil {
			return nil, err
		}
		row.TargetName, row.TargetMissing = target.Name, false
		out = append(out, row)
		seen[target.Kind+"\x00"+target.ID] = true
	}
	rows, err := s.ScheduledUpdatePolicies(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.EnvironmentID != env || seen[row.TargetKind+"\x00"+row.TargetID] {
			continue
		}
		one, err := s.GetImagePolicy(ctx, env, row.TargetID, row.TargetKind)
		if err != nil {
			return nil, err
		}
		one.TargetMissing = true
		out = append(out, one)
	}
	return out, nil
}

func imageCheckInfo(r *checkDTO.Response) *imageDTO.UpdateInfo {
	return &imageDTO.UpdateInfo{State: r.State, CheckedAt: r.CheckedAt, ImageRef: r.ImageRef, Platform: r.Platform, AuthState: r.AuthState, Code: r.Code, HasUpdate: r.HasUpdate, CurrentVersion: r.CurrentVersion, LatestVersion: r.LatestVersion, CurrentDigest: r.CurrentDigest, LatestDigest: r.LatestDigest}
}

func projectUpdateTargets(stacks []projectDTO.Details, inventory []containerDTO.Summary) ([]updateTarget, error) {
	out := make([]updateTarget, 0, len(stacks)+len(inventory))
	for _, stack := range stacks {
		out = append(out, updateTarget{Kind: "stack", ID: stack.ID, Name: stack.Name, Eligible: true})
	}
	for _, row := range inventory {
		if len(row.Names) == 0 || row.ID == "" {
			return nil, imageDTO.Failure("update.check-unavailable", 503)
		}
		name := strings.TrimPrefix(row.Names[0], "/")
		out = append(out, updateTarget{Kind: "container", ID: name, Name: name, ContainerID: row.ID, ImageRef: row.Image, Platform: row.ImagePlatform, Eligible: row.Origin == containerDTO.OriginStandalone && row.OriginSource != containerDTO.OriginUnconfirmed && !row.RedeployDisabled, AutoExcluded: !row.AutoUpdateEnabled})
	}
	return out, nil
}
