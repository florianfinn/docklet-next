// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original split of proxy wiring options to retain the file size bound.

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
