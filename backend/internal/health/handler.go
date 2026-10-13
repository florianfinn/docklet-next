// Source: getarcaneapp/arcane/backend/internal/health/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package health

import (
	"context"

	"github.com/florianfinn/docklet-next/types/system"
)

type healthHandler struct{}

type healthOutput struct {
	Body system.HealthResponse
}

func (h *healthHandler) getHealth(context.Context, *struct{}) (*healthOutput, error) {
	return &healthOutput{
		Body: system.HealthResponse{
			Status: "UP",
		},
	}, nil
}

func (h *healthHandler) headHealth(context.Context, *struct{}) (*struct{}, error) {
	return nil, nil
}
