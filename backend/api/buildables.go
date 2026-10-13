// Source: getarcaneapp/arcane/backend/api/buildables.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed OIDC and passkeys.

//go:build buildables

package api

import (
	"errors"
	"net/http"
	"time"

	authtypes "github.com/florianfinn/docklet-next/types/auth"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"go.getarcane.app/kit/pkg/mapping"

	"github.com/florianfinn/docklet-next/backend/buildables"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
)

// SetupBuildablesRoutes registers buildable feature routes based on EnabledFeatures.
func SetupBuildablesRoutes(apiGroup *echo.Group, authService *auth.AuthService) {
	if buildables.HasBuildFeature("autologin") {
		registerAutoLoginRoutes(apiGroup, authService)
	}
}

func registerAutoLoginRoutes(apiGroup *echo.Group, authService *auth.AuthService) {
	authGroup := apiGroup.Group("/auth")

	authGroup.GET("/auto-login-config", func(c *echo.Context) error {
		if authService == nil {
			return c.JSON(http.StatusInternalServerError, base.ErrorResponse{Error: "service not available"})
		}

		config, err := authService.GetAutoLoginConfig(c.Request().Context())
		if err != nil {
			return c.JSON(http.StatusInternalServerError, base.ErrorResponse{Error: "failed to get auto-login config"})
		}

		return c.JSON(http.StatusOK, base.ApiResponse[authtypes.AutoLoginConfig]{
			Success: true,
			Data:    *config,
		})
	})

	authGroup.POST("/auto-login", func(c *echo.Context) error {
		if authService == nil {
			return c.JSON(http.StatusInternalServerError, base.ErrorResponse{Error: "service not available"})
		}

		autoLoginConfig, err := authService.GetAutoLoginConfig(c.Request().Context())
		if err != nil {
			return c.JSON(http.StatusInternalServerError, base.ErrorResponse{Error: "failed to get auto-login config"})
		}

		if !autoLoginConfig.Enabled {
			return c.JSON(http.StatusBadRequest, base.ErrorResponse{Error: "auto-login is not enabled"})
		}

		password := authService.GetAutoLoginPassword()
		if password == "" {
			return c.JSON(http.StatusInternalServerError, base.ErrorResponse{Error: "auto-login password not configured"})
		}

		userModel, tokenPair, err := authService.Login(c.Request().Context(), autoLoginConfig.Username, password, authtypes.SessionMeta{
			UserAgent: c.Request().UserAgent(),
			IPAddress: c.RealIP(),
		})
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrInvalidCredentials):
				return c.JSON(http.StatusUnauthorized, base.ErrorResponse{Error: "Invalid username or password"})
			case errors.Is(err, auth.ErrLocalAuthDisabled):
				return c.JSON(http.StatusBadRequest, base.ErrorResponse{Error: "Local authentication is disabled"})
			default:
				return c.JSON(http.StatusInternalServerError, base.ErrorResponse{Error: "Authentication failed"})
			}
		}

		var userResp user.User
		if mapErr := mapping.MapStruct(userModel, &userResp); mapErr != nil {
			return c.JSON(http.StatusInternalServerError, base.ErrorResponse{Error: "Failed to map user"})
		}

		maxAge := max(int(time.Until(tokenPair.ExpiresAt).Seconds()), 0)
		maxAge += 60
		expiresAt := tokenPair.ExpiresAt

		for _, tokenCookie := range cookie.BuildTokenCookieStringFor(maxAge, tokenPair.BrowserToken, cookie.SecureCookieFromRequest(c.Request())) {
			c.Response().Header().Add("Set-Cookie", tokenCookie)
		}
		return c.JSON(http.StatusOK, base.ApiResponse[authtypes.AuthenticationResponse]{
			Success: true,
			Data: authtypes.AuthenticationResponse{
				Status:       authtypes.AuthenticationStatusAuthenticated,
				Token:        tokenPair.AccessToken,
				RefreshToken: tokenPair.RefreshToken,
				ExpiresAt:    new(expiresAt),
				User:         new(userResp),
			},
		})
	})
}
