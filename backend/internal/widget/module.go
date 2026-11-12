// Source: docklet-next/issues/332
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original hub-only widget HTTP module and neutral validation errors.

package widget

import (
	"context"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	dto "github.com/florianfinn/docklet-next/types/widget"
)

type Module struct{ service *Service }

func New(service *Service) *Module { return &Module{service: service} }

type kindInput struct {
	Kind dto.Kind `path:"kind" enum:"plex,overseerr,minecraft"`
}
type configInput struct {
	Kind dto.Kind `path:"kind" enum:"plex,overseerr,minecraft"`
	Body dto.ConfigUpdate
}
type requestInput struct {
	ID int64 `path:"requestId" minimum:"1"`
}
type output[T any] struct{ Body T }

func (m *Module) RegisterRoutes(api huma.API) {
	var s *Service
	if m != nil {
		s = m.service
	}
	register(api, "list-service-widgets", http.MethodGet, "/service-widgets", func(ctx context.Context, _ *struct{}) (*output[[]dto.Summary], error) {
		v, err := s.List(ctx)
		return &output[[]dto.Summary]{Body: v}, err
	})
	register(api, "get-service-widget", http.MethodGet, "/service-widgets/{kind}", func(ctx context.Context, in *kindInput) (*output[dto.Result], error) {
		return &output[dto.Result]{Body: s.Get(ctx, in.Kind)}, nil
	})
	register(api, "get-service-widget-config", http.MethodGet, "/service-widgets/{kind}/config", func(ctx context.Context, in *kindInput) (*output[dto.Config], error) {
		v, err := s.Config(ctx, in.Kind)
		return &output[dto.Config]{Body: v}, err
	})
	register(api, "put-service-widget-config", http.MethodPut, "/service-widgets/{kind}/config", func(ctx context.Context, in *configInput) (*output[dto.Config], error) {
		v, err := s.Save(ctx, in.Kind, in.Body)
		return &output[dto.Config]{Body: v}, err
	})
	register(api, "test-service-widget", http.MethodPost, "/service-widgets/{kind}/test", func(ctx context.Context, in *configInput) (*output[dto.Result], error) {
		v, err := s.Test(ctx, in.Kind, in.Body)
		return &output[dto.Result]{Body: v}, err
	})
	for _, action := range []string{"approve", "decline"} {
		register(api, action+"-service-widget-request", http.MethodPost, "/service-widgets/overseerr/requests/{requestId}/"+action, func(ctx context.Context, in *requestInput) (*output[dto.ActionResult], error) {
			actor, err := handlerutil.RequireUser(ctx)
			if err != nil {
				return nil, err
			}
			v, err := s.Action(ctx, in.ID, action, actor)
			return &output[dto.ActionResult]{Body: v}, err
		})
	}
}

func register[I, O any](api huma.API, id, method, path string, handler func(context.Context, *I) (*O, error)) {
	schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.Problem](), true, "")
	responses := map[string]*huma.Response{}
	for _, status := range []string{"400", "503"} {
		responses[status] = &huma.Response{Description: "Neutral widget failure", Content: map[string]*huma.MediaType{"application/json": {Schema: schema}}}
	}
	huma.Register(api, huma.Operation{OperationID: id, Method: method, Path: path, Tags: []string{"Service widgets"}, Responses: responses, Security: handlerutil.DefaultOperationSecurity(), MaxBodyBytes: 16 << 10, Middlewares: huma.Middlewares{func(ctx huma.Context, next func(huma.Context)) {
		ps, ok := middleware.PermissionsFromContext(ctx.Context())
		// Administrator permission sets carry global grants without UserID;
		// simple-user sets carry their local user ID and assignments.
		if !ok || ps == nil || (ps.UserID == "" && !ps.IsGlobalAdmin()) {
			_ = huma.WriteErr(api, ctx, 401, "Not authenticated")
			return
		}
		if access.Rules[id].Kind != access.Self && !ps.IsGlobalAdmin() {
			_ = huma.WriteErr(api, ctx, 403, "Administrator required")
			return
		}
		next(ctx)
	}}}, handler)
}

// ErrorTransformer prevents Huma validation diagnostics from echoing secrets.
// It is installed for both schema export and the actual API configuration.
func ErrorTransformer(ctx huma.Context, status string, value any) (any, error) {
	if !strings.HasPrefix(strings.TrimPrefix(ctx.URL().Path, "/api"), "/service-widgets") {
		return value, nil
	}
	n, _ := strconv.Atoi(status)
	if n < 400 || n == 401 || n == 403 {
		return value, nil
	}
	if p, ok := value.(*dto.Problem); ok {
		p.Certificate = nil
		return p, nil
	}
	if n == 400 || n == 422 || n == 413 {
		ctx.SetStatus(400)
		return dto.Failure(dto.InvalidConfig), nil
	}
	return dto.Failure(dto.PersistenceFailed), nil
}
