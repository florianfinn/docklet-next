// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original mode-aware history producer and hub read routes.

package history

import (
	"context"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/history"
	"reflect"
	"time"
)

type Module struct {
	service  *Service
	producer *Producer
}

func New(s *Service, p *Producer) *Module { return &Module{s, p} }

type SampleInput struct {
	EnvironmentID string `path:"id"`
}
type ReadInput struct {
	EnvironmentID string `path:"id"`
	Range         string `query:"range" default:"24h"`
}
type ContainerInput struct {
	EnvironmentID string `path:"id"`
	ContainerID   string `path:"containerId" doc:"Container name, preserved across recreation"`
	Range         string `query:"range" default:"24h"`
}
type ProjectInput struct {
	EnvironmentID string `path:"id"`
	ProjectID     string `path:"projectId"`
	Range         string `query:"range" default:"24h"`
}

func out[T any](value T, err error) (*handlerutil.Out[T], error) {
	if err != nil {
		if p, ok := errors.AsType[*dto.Problem](err); ok {
			return nil, huma.NewError(p.Status, p.Code)
		}
		return nil, huma.Error503ServiceUnavailable("history.unavailable")
	}
	return &handlerutil.Out[T]{Body: base.ApiResponse[T]{Success: true, Data: value}}, nil
}
func (m *Module) RegisterRoutes(api huma.API, agent bool) {
	api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.Problem](), true, "HistoryProblem")
	metadata := []*huma.Schema{}
	for _, typ := range []reflect.Type{reflect.TypeFor[dto.ConditionEventMetadata](), reflect.TypeFor[dto.LoadAlertMetadata]()} {
		metadata = append(metadata, api.OpenAPI().Components.Schemas.Schema(typ, true, ""))
	}
	if api.OpenAPI().Extensions == nil {
		api.OpenAPI().Extensions = map[string]any{}
	}
	api.OpenAPI().Extensions["x-docklet-history-v13"] = metadata
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "get-history-sample", Method: "GET", Path: "/environments/{id}/history/sample", Summary: "Collect an owning-host runtime history sample", Tags: []string{"History"}, Security: handlerutil.DefaultOperationSecurity()}, authz.PermSystemRead, func(ctx context.Context, input *SampleInput) (*handlerutil.Out[dto.Sample], error) {
		return out(m.producer.Sample(ctx))
	})
	if agent {
		return
	}
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "get-container-history", Method: "GET", Path: "/environments/{id}/containers/{containerId}/history", Summary: "Read container history by name", Tags: []string{"History"}, Security: handlerutil.DefaultOperationSecurity()}, authz.PermContainersRead, func(ctx context.Context, i *ContainerInput) (*handlerutil.Out[dto.Response], error) {
		return out(readHistory(ctx, m.service.db, "container", i.EnvironmentID, i.ContainerID, i.Range, time.Now().UTC()))
	})
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "get-project-history", Method: "GET", Path: "/environments/{id}/projects/{projectId}/history", Summary: "Read project runtime history", Tags: []string{"History"}, Security: handlerutil.DefaultOperationSecurity()}, authz.PermProjectsRead, func(ctx context.Context, i *ProjectInput) (*handlerutil.Out[dto.Response], error) {
		return out(readHistory(ctx, m.service.db, "stack", i.EnvironmentID, i.ProjectID, i.Range, time.Now().UTC()))
	})
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "get-host-history", Method: "GET", Path: "/environments/{id}/history/host", Summary: "Read host resource history", Tags: []string{"History"}, Security: handlerutil.DefaultOperationSecurity()}, authz.PermSystemRead, func(ctx context.Context, i *ReadInput) (*handlerutil.Out[dto.Response], error) {
		return out(readHistory(ctx, m.service.db, "host", i.EnvironmentID, i.EnvironmentID, i.Range, time.Now().UTC()))
	})
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "get-runtime-conditions", Method: "GET", Path: "/environments/{id}/runtime-conditions", Summary: "Read current assigned runtime conditions", Tags: []string{"History"}, Security: handlerutil.DefaultOperationSecurity()}, authz.PermProjectsList, func(ctx context.Context, i *SampleInput) (*handlerutil.Out[dto.Conditions], error) {
		result, err := m.service.Conditions(ctx, i.EnvironmentID)
		ps, _ := middleware.PermissionsFromContext(ctx)
		principal := access.FromPermissions(ps)
		if principal.IsUser() {
			allowed := map[string]bool{}
			for _, id := range principal.ProjectIDs(i.EnvironmentID) {
				allowed[id] = true
			}
			filtered := []dto.Condition{}
			for _, c := range result.Items {
				if allowed[c.ProjectID] {
					filtered = append(filtered, c)
				}
			}
			result.Items = filtered
		}
		return out(result, err)
	})
	huma.Register(api, huma.Operation{OperationID: "get-history-capacity", Method: "GET", Path: "/history/capacity", Summary: "Read host capacity trends in bytes per day", Tags: []string{"History"}, Security: handlerutil.DefaultOperationSecurity()}, func(ctx context.Context, _ *struct{}) (*handlerutil.Out[[]dto.Capacity], error) {
		return out(m.service.Capacity(ctx, time.Now().UTC()))
	})
}
func (s *Service) Capacity(ctx context.Context, now time.Time) ([]dto.Capacity, error) {
	rows := []bucketRow{}
	if err := s.db.WithContext(ctx).Where("kind='host' AND tier=120 AND bucket_start>=?", now.Add(-7*24*time.Hour).Unix()).Order("host_id,bucket_start").Find(&rows).Error; err != nil {
		return nil, err
	}
	groups := map[string][]bucketRow{}
	for _, r := range rows {
		groups[r.HostID] = append(groups[r.HostID], r)
	}
	result := []dto.Capacity{}
	for id, series := range groups {
		var enabled int64
		if err := s.db.WithContext(ctx).Table("environments").Where("id=? AND enabled=?", id, true).Count(&enabled).Error; err != nil {
			return nil, err
		}
		if enabled == 0 {
			continue
		}
		// The newest bucket with a measured disk; a host that is down right now keeps its last usage and trend.
		measured := len(series) - 1
		for measured >= 0 && (series[measured].MinutesRunning == 0 || series[measured].DiskSource == "") {
			measured--
		}
		if measured < 0 {
			continue
		}
		current := series[measured]
		c := capacityGrowth(series, current)
		var err error
		c.History, err = readHistory(ctx, s.db, "host", id, id, "30d", now)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, nil
}
