// Source: getarcaneapp/arcane/backend/internal/middleware/environment.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Split retained hub proxy wiring options from environment.go; add the shared shell registry; M18 live scheduling, policy and contract foundation; optional-module foundation (#472); M29 runtime proxy identity header.

package middleware

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/containershell"
	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/types/features"
	"net/http"
	"strings"
)

// EnvironmentProxySecurity supplies hub-owned identity and agent access boundaries.
type EnvironmentProxySecurity struct {
	ModuleEnabled              func(context.Context, features.ID) bool
	ShellSessions              *containershell.Registry
	VulnerabilityEnabled       func(context.Context) bool
	VulnerabilityIgnoreUnfixed func() bool
	PointHolds                 PointHoldReader
	PointAccess                PointAccessReader
	PolicyCleanup              func(context.Context, string, string) error
	OriginOverlay              func(context.Context, string, []byte) ([]byte, error)
	Snapshots                  *hoststate.Store
	InventoryPending           func(context.Context, string) (bool, error)
	HTTPClient                 *http.Client
	ProxyIdentityHeader        string
	// ProxyIdentityHeaderFunc follows runtime sign-in changes; it takes precedence over ProxyIdentityHeader.
	ProxyIdentityHeaderFunc func() string
	AgentAccess             remenv.AgentAccessCheck
}

// accessAuthStatusInternal: a pending first-run setup is unauthenticated (401); access refusals are 403.
func accessAuthStatusInternal(code string) int {
	if strings.HasPrefix(code, "setup.") {
		return http.StatusUnauthorized
	}
	return http.StatusForbidden
}
