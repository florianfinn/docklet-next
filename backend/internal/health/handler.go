// Source: getarcaneapp/arcane/backend/internal/health/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; returns 503 once the application lifecycle has ended.; M14 container and host operation contracts.

package health

import (
	"context"
	"github.com/google/uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/types/system"
)

// processInstanceID is generated once per process, independently of database identity.
var processInstanceID = uuid.NewString()

// healthHandler reports UP only while live reports the application lifecycle as running.
type healthHandler struct{ live func() bool }

func (h *healthHandler) unavailable() error {
	if h.live != nil && !h.live() {
		return huma.Error503ServiceUnavailable("application is stopping")
	}
	return nil
}

type healthOutput struct {
	Body system.HealthResponse
}

func (h *healthHandler) getHealth(context.Context, *struct{}) (*healthOutput, error) {
	if err := h.unavailable(); err != nil {
		return nil, err
	}
	return &healthOutput{
		Body: system.HealthResponse{
			Status:     "UP",
			InstanceID: processInstanceID,
		},
	}, nil
}

func (h *healthHandler) headHealth(context.Context, *struct{}) (*struct{}, error) {
	return nil, h.unavailable()
}
