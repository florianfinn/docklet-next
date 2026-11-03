// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original history state invalidation wiring.

package di

import (
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/history"
	"github.com/florianfinn/docklet-next/backend/internal/system"
)

func wireHistory(e *environment.EnvironmentService, h *history.Service, p *history.Producer, systemService *system.SystemService) {
	e.SetHistoryInvalidator(h.Forget)
	p.SetDockerHostMemoryProvider(systemService.GetDockerHostMemory)
}
