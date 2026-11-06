// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original hub import contract and resolver boundary.

package stackimport

import (
	"context"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	dto "github.com/florianfinn/docklet-next/types/stackimport"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"unicode/utf8"
)

// Resolver returns an unsaved proposal; implementations must not persist or deploy it.
type Resolver interface {
	Resolve(context.Context, dto.Request) (dto.Result, error)
}
type unavailableResolver struct{}

func (unavailableResolver) Resolve(context.Context, dto.Request) (dto.Result, error) {
	return dto.Result{}, dto.Failure("import.source-unavailable", 502)
}

type Module struct {
	resolver Resolver
	flights  sync.Map
}

func New() *Module                       { return NewWithResolver(unavailableResolver{}) }
func NewWithResolver(r Resolver) *Module { return &Module{resolver: r} }

type input struct{ Body dto.Request }
type output struct{ Body dto.Result }

func (m *Module) RegisterRoutes(api huma.API) {
	if m == nil {
		m = New()
	}
	huma.Register(api, huma.Operation{OperationID: "import-stack-source", Method: http.MethodPost, Path: "/stack-import", Summary: "Import an unsaved stack proposal", Tags: []string{"Stacks"}, Security: handlerutil.DefaultOperationSecurity(), SkipValidateBody: true, Middlewares: huma.Middlewares{func(c huma.Context, next func(huma.Context)) {
		ps, _ := middleware.PermissionsFromContext(c.Context())
		if !ps.IsGlobalAdmin() {
			_ = access.WriteProblem(c, 403, "access.admin-required")
			return
		}
		next(c)
	}}}, m.importSource)
	ref := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.Problem](), true, "")
	op := api.OpenAPI().Paths["/stack-import"].Post
	for _, status := range []string{"400", "404", "409", "413", "422", "502", "504"} {
		op.Responses[status] = &huma.Response{Description: "Import refusal", Content: map[string]*huma.MediaType{"application/problem+json": {Schema: ref}}}
	}
}
func (m *Module) importSource(ctx context.Context, in *input) (*output, error) {
	principal, ok := middleware.GetUserIDFromContext(ctx)
	if !ok || principal == "" {
		return nil, access.Problem(401, "access.authentication-required")
	}
	if in.Body.Kind != "github" && in.Body.Kind != "registry" || strings.TrimSpace(in.Body.Ref) == "" || utf8.RuneCountInString(in.Body.Ref) > 512 {
		return nil, dto.Failure("import.invalid-reference", 400)
	}
	if _, busy := m.flights.LoadOrStore(principal, true); busy {
		return nil, dto.Failure("import.busy", 409)
	}
	defer m.flights.Delete(principal)
	result, err := m.resolver.Resolve(ctx, in.Body)
	if err != nil {
		if p, ok := errors.AsType[*dto.Problem](err); ok {
			return nil, p
		}
		return nil, dto.Failure("import.source-unavailable", 502)
	}
	return &output{Body: result}, nil
}
