// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original validated machine transport CSRF exception.

package bootstrap

import (
	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"net/http"
	"strings"
)

func csrfTransportValidatorInternal(deps api.HandlerDeps) func(*http.Request) bool {
	return func(req *http.Request) bool {
		if deps.ApiKey != nil {
			if key := req.Header.Get(middleware.HeaderApiKey); key != "" {
				if envID, err := deps.ApiKey.Service().GetEnvironmentByApiKey(req.Context(), key); err == nil && envID != nil {
					return true
				}
			}
		}
		if deps.Environment == nil {
			return false
		}
		for _, header := range []string{middleware.HeaderAgentToken, middleware.HeaderApiKey} {
			if token := req.Header.Get(header); token != "" {
				if env, err := deps.Environment.Service().ResolveEnvironmentByAccessToken(req.Context(), token); err == nil && env != nil {
					return true
				}
			}
		}
		if strings.HasPrefix(req.URL.Path, "/api/tunnel/") {
			if token, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer "); ok {
				envID, err := deps.Environment.Service().ResolveEdgeEnvironmentByToken(req.Context(), token)
				return err == nil && envID != ""
			}
		}
		return false
	}
}
