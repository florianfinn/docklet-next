// Source: docklet-next/issues/68
// Revision: af5fa9df01e730f7691d650985abd33d74f51ae5
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original validation of explicit local credentials for browser-protection exceptions.

package auth

import (
	"net/http"
	"strings"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
)

// ValidateLocalHeaderCredential never falls back to browser cookies or proxy identity.
// An invalid explicit credential returns an error so CLI clients can refresh after 401.
func (m *AuthMiddleware) ValidateLocalHeaderCredential(req *http.Request) (bool, error) {
	if m.authService == nil || m.authService.SignInMode() != "local" {
		return false, nil
	}
	if key := req.Header.Get(middleware.HeaderApiKey); key != "" {
		if m.apiKeyValidator != nil {
			actor, _, err := m.apiKeyValidator.ValidateApiKeyWithID(req.Context(), key)
			if err == nil && actor != nil {
				return true, nil
			}
		}
		return false, common.ErrInvalidToken
	}
	if token, ok := BearerToken(req.Header.Get("Authorization")); ok {
		actor, _, err := m.authService.VerifyToken(req.Context(), token)
		if err != nil || actor == nil {
			return false, common.ErrInvalidToken
		}
		return true, nil
	}
	return false, nil
}

// BearerToken keeps credential selection identical before and after origin checks.
func BearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	return token, ok && strings.EqualFold(scheme, "Bearer")
}

// ValidateNonCookieCredential also preserves authenticated agent stream transports.
func (m *AuthMiddleware) ValidateNonCookieCredential(req *http.Request) bool {
	if m.cfg != nil && (m.cfg.AgentMode || m.cfg.EdgeAgent) {
		return AgentTokenMatches(req.Header.Get(middleware.HeaderAgentToken), m.cfg.AgentToken) ||
			AgentTokenMatches(req.Header.Get(middleware.HeaderApiKey), m.cfg.AgentToken)
	}
	for _, header := range []string{middleware.HeaderAgentToken, middleware.HeaderApiKey} {
		if _, ok := m.resolveEnvironmentAccessToken(req.Context(), req.Header.Get(header)).Get(); ok {
			return true
		}
	}
	valid, _ := m.ValidateLocalHeaderCredential(req)
	return valid
}
