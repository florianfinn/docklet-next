// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package bootstrap

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/types/base"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	"net/http"
	"net/url"
)

func resourceAccessController(deps api.HandlerDeps) *access.Controller {
	return &access.Controller{
		Runtime: func(ctx context.Context, env, id string) ([]access.Container, error) {
			var details projectDTO.Details
			var err error
			if env == "0" {
				details, err = deps.Project.Service().GetProjectDetails(ctx, id, projectDTO.DetailsOptions{IncludeRuntimeServices: true})
			} else {
				var response base.ApiResponse[projectDTO.Details]
				err = deps.Environment.Service().ProxyJSONRequest(ctx, env, http.MethodGet, "/api/environments/0/projects/"+url.PathEscape(id)+"/runtime", nil, &response)
				details = response.Data
			}
			if err != nil {
				return nil, err
			}
			out := []access.Container{}
			for _, v := range details.RuntimeServices {
				if v.ContainerID != "" {
					out = append(out, access.Container{ID: v.ContainerID, Names: []string{v.ContainerName}})
				}
			}
			return out, nil
		},
		Projects: func(ctx context.Context, env string, filters url.Values) ([]projectDTO.Details, error) {
			if env == "0" {
				f := map[string]string{}
				for k := range filters {
					f[k] = filters.Get(k)
				}
				rows, _, err := deps.Project.Service().ListProjects(ctx, pagination.QueryParams{Params: pagination.Params{Limit: -1}, Filters: f})
				return rows, err
			}
			filters.Set("limit", "-1")
			var response base.Paginated[projectDTO.Details]
			if err := deps.Environment.Service().ProxyJSONRequest(ctx, env, http.MethodGet, "/api/environments/0/projects?"+filters.Encode(), nil, &response); err != nil {
				return nil, err
			}
			return response.Data, nil
		},
		RunOwner: func(ctx context.Context, env, run string) (string, error) {
			ref, found, err := deps.Runtime.Reference(ctx, env, run)
			if err != nil {
				return "", err
			}
			if !found {
				return "", errors.New("run not found")
			}
			detail, err := deps.Runtime.GetRuntimeDetail(ctx, ref)
			if err != nil {
				return "", err
			}
			if detail.Activity.StartedBy == nil {
				return "", errors.New("run owner unavailable")
			}
			return detail.Activity.StartedBy.UserID, nil
		},
	}
}
