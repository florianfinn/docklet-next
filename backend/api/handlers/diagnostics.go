// Source: getarcaneapp/arcane/backend/api/handlers/diagnostics.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package handlers

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/types/system"

	"github.com/florianfinn/docklet-next/backend/internal/diagnostics"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// DiagnosticsHandler serves the environment-scoped actor diagnostics the manager
// collects from each agent. The diagnostics page itself uses the api/ws streams.
type DiagnosticsHandler struct {
	diag *diagnostics.DiagnosticsService
}

type GetEnvironmentActorDiagnosticsInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
}

type GetEnvironmentActorDiagnosticsOutput struct {
	Body system.ActorDiagnostics
}

// RegisterDiagnostics registers the Huma diagnostics REST endpoints.
func RegisterDiagnostics(api huma.API, diag *diagnostics.DiagnosticsService) {
	h := &DiagnosticsHandler{diag: diag}

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "get-environment-actor-diagnostics",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/diagnostics/actors",
		Summary:     "Get actor diagnostics for an environment",
		Description: "Returns Francis actor host, actor type, and run queue diagnostics for one environment.",
		Tags:        []string{"Diagnostics"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermDiagnosticsRead, h.GetEnvironmentActorDiagnostics)
}

func (h *DiagnosticsHandler) GetEnvironmentActorDiagnostics(ctx context.Context, _ *GetEnvironmentActorDiagnosticsInput) (*GetEnvironmentActorDiagnosticsOutput, error) {
	d, err := h.diag.CollectActors(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to collect actor diagnostics: " + err.Error())
	}
	return &GetEnvironmentActorDiagnosticsOutput{Body: d}, nil
}
