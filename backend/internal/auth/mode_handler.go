// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original read-only mode endpoint; M15 simple-user rights and identity contracts; M29 setup state and stored policy.

package auth

import (
	"context"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/types/auth"
	"github.com/florianfinn/docklet-next/types/base"
)

type SignInModeOutput struct {
	CSRFToken string `header:"X-CSRF-Token" doc:"CSRF header token; send with the matching cookie and hub Origin on writes"`
	Body      base.ApiResponse[auth.SignInConfiguration]
}

func (h *AuthHandler) GetSignInMode(ctx context.Context, _ *struct{}) (*SignInModeOutput, error) {
	p := h.authService.CurrentPolicy()
	cfg := auth.SignInConfiguration{Mode: p.Mode, SetupPending: p.Pending, Forced: p.Forced}
	if cfg.Mode == auth.SignInModeProxy && h.authService.config.AuthProxyLogoutURL != "" {
		cfg.LogoutURL = &h.authService.config.AuthProxyLogoutURL
	}
	// The public endpoint shows proxy settings only during setup; administrators read them via the
	// authenticated Sign-in section (this operation is public, so no user is resolved here).
	if p.Pending {
		cfg.ProxyHeader, cfg.TrustedProxies = &p.Header, &p.Trusted
	}
	return &SignInModeOutput{
		CSRFToken: middleware.CSRFTokenFromContext(ctx),
		Body:      base.ApiResponse[auth.SignInConfiguration]{Success: true, Data: cfg},
	}, nil
}
