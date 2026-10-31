// Source: getarcaneapp/arcane/backend/internal/middleware/environment.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Split retained hub proxy wiring options from environment.go; add the shared shell registry.

package middleware

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/containershell"
	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"net/http"
)

// EnvironmentProxySecurity supplies hub-owned identity and agent access boundaries.
type EnvironmentProxySecurity struct {
	ShellSessions              *containershell.Registry
	VulnerabilityEnabled       func(context.Context) bool
	VulnerabilityIgnoreUnfixed func() bool
	PointHolds                 PointHoldReader
	PointAccess                PointAccessReader
	OriginOverlay              func(context.Context, string, []byte) ([]byte, error)
	Snapshots                  *hoststate.Store
	InventoryPending           func(context.Context, string) (bool, error)
	HTTPClient                 *http.Client
	ProxyIdentityHeader        string
	AgentAccess                remenv.AgentAccessCheck
}
