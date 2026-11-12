// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original read-only mode endpoint; M15 simple-user rights and identity contracts.

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
	cfg := auth.SignInConfiguration{Mode: h.authService.SignInMode()}
	if cfg.Mode == auth.SignInModeProxy && h.authService.config.AuthProxyLogoutURL != "" {
		cfg.LogoutURL = &h.authService.config.AuthProxyLogoutURL
	}
	return &SignInModeOutput{
		CSRFToken: middleware.CSRFTokenFromContext(ctx),
		Body:      base.ApiResponse[auth.SignInConfiguration]{Success: true, Data: cfg},
	}, nil
}
