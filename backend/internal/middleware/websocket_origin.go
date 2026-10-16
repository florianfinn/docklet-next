// Source: docklet-next/issues/68
// Revision: af5fa9df01e730f7691d650985abd33d74f51ae5
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original shared HTTP and WebSocket browser-origin policy.

package middleware

import (
	"net/http"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/httpx"
)

func WebSocketOriginValidator(cfg *config.Config, credentialAuthenticated func(*http.Request) bool) func(*http.Request) bool {
	return httpx.ValidateWebSocketOrigin(deriveAllowedOriginsInternal(cfg, nil), credentialAuthenticated)
}
