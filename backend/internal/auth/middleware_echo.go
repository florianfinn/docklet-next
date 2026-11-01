// Source: getarcaneapp/arcane/backend/internal/auth/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Split Echo middleware; local module namespace; removed OIDC/passkeys; startup sign-in modes and shared reserved synthetic agent identity; session-bound viewer lifetime and durable validation; M15 simple-user rights and identity contracts.

package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	userdomain "github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	"github.com/labstack/echo/v5"
	"github.com/samber/mo"
	"log/slog"
	"net/http"
	"strings"
)

type AuthOptions struct {
	AdminRequired   bool
	SuccessOptional bool
}

type ApiKeyValidator interface {
	ValidateApiKeyWithID(ctx context.Context, rawKey string) (*userdomain.User, *apikey.ApiKey, error)
}

type EnvironmentAccessTokenResolver interface {
	ResolveEnvironmentByAccessToken(ctx context.Context, token string) (*environment.Environment, error)
}

// PermissionResolver resolves a caller's effective permission set. Implemented
// by role.RoleService; kept as an interface so tests can stub it.
type PermissionResolver interface {
	ResolvePermissions(ctx context.Context, userID string) (*authz.PermissionSet, error)
	ResolveApiKeyPermissions(ctx context.Context, apiKeyID string) (*authz.PermissionSet, error)
}

type AuthMiddleware struct {
	authService      *AuthService
	apiKeyValidator  ApiKeyValidator
	envTokenResolver EnvironmentAccessTokenResolver
	roleResolver     PermissionResolver
	cfg              *config.Config
	options          AuthOptions
}

func NewAuthMiddleware(authService *AuthService, cfg *config.Config) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
		cfg:         cfg,
		options:     AuthOptions{},
	}
}

func (m *AuthMiddleware) WithApiKeyValidator(validator ApiKeyValidator) *AuthMiddleware {
	clone := *m
	clone.apiKeyValidator = validator
	return &clone
}

func (m *AuthMiddleware) WithEnvironmentAccessTokenResolver(resolver EnvironmentAccessTokenResolver) *AuthMiddleware {
	clone := *m
	clone.envTokenResolver = resolver
	return &clone
}

func (m *AuthMiddleware) WithPermissionResolver(resolver PermissionResolver) *AuthMiddleware {
	clone := *m
	clone.roleResolver = resolver
	return &clone
}

func (m *AuthMiddleware) WithAdminNotRequired() *AuthMiddleware {
	clone := *m
	clone.options.AdminRequired = false
	return &clone
}

func (m *AuthMiddleware) WithAdminRequired() *AuthMiddleware {
	clone := *m
	clone.options.AdminRequired = true
	return &clone
}

func (m *AuthMiddleware) Add() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			reqCtx := c.Request().Context()
			if m.cfg != nil && m.cfg.AgentMode {
				return m.agentAuth(reqCtx, c, next)
			}
			return m.managerAuth(reqCtx, c, next)
		}
	}
}

func (m *AuthMiddleware) agentAuth(ctx context.Context, c *echo.Context, next echo.HandlerFunc) error {
	if isPreflightInternal(c) {
		return next(c)
	}

	req := c.Request()
	if strings.HasPrefix(req.URL.Path, middleware.AgentPairingPrefix) &&
		AgentTokenMatches(req.Header.Get(middleware.HeaderAgentBootstrap), m.cfg.AgentToken) {
		slog.InfoContext(ctx, "Agent auth: bootstrap pairing accepted", "path", req.URL.Path, "method", req.Method)
		agentSudoInternal(c)
		return next(c)
	}

	if AgentTokenMatches(req.Header.Get(middleware.HeaderAgentToken), m.cfg.AgentToken) {
		agentSudoInternal(c)
		return next(c)
	}

	// Check for API key as agent token
	if AgentTokenMatches(req.Header.Get(middleware.HeaderApiKey), m.cfg.AgentToken) {
		agentSudoInternal(c)
		return next(c)
	}

	slog.WarnContext(
		ctx, "Agent auth forbidden",
		"path", req.URL.Path,
		"method", req.Method,
		"hasAgentTokenHdr", req.Header.Get(middleware.HeaderAgentToken) != "",
		"agentTokenConfigSet", m.cfg.AgentToken != "",
	)
	return c.JSON(http.StatusForbidden, common.APIError{
		Code:    "FORBIDDEN",
		Message: "Invalid or missing agent token",
	})
}

// AgentTokenMatches compares a presented token against the configured
// agent token in constant time to avoid timing side channels.
func AgentTokenMatches(presented, configured string) bool {
	if presented == "" || configured == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(configured)) == 1
}

func (m *AuthMiddleware) managerAuth(ctx context.Context, c *echo.Context, next echo.HandlerFunc) error {
	req := c.Request()
	if agentToken := req.Header.Get(middleware.HeaderAgentToken); agentToken != "" {
		if env, ok := m.resolveEnvironmentAccessToken(ctx, agentToken).Get(); ok {
			ps := environmentScopedInternal(c, env)
			return m.authorizeAndContinueInternal(c, next, ps)
		}
	}

	// First, check for API key in X-API-Key header
	if m.authService.SignInMode() != "local" {
		if env, ok := m.resolveEnvironmentAccessToken(ctx, req.Header.Get(middleware.HeaderApiKey)).Get(); ok {
			return m.authorizeAndContinueInternal(c, next, environmentScopedInternal(c, env))
		}
		requestUser, err := m.authService.ModeRequestUser(ctx, req)
		if err != nil {
			status, code := modeAuthError(err)
			return c.JSON(status, common.APIError{Code: common.APIErrorCode(code), Message: code})
		}
		ps := m.authService.modePermissions(ctx, requestUser)
		c.SetRequest(req.WithContext(m.authService.ViewerContextForPermissions(req.Context(), "", requestUser.ID, ps)))
		c.Set(string(middleware.ContextKeyUserID), requestUser.ID)
		c.Set(string(middleware.ContextKeyCurrentUser), requestUser)
		c.Set(string(middleware.ContextKeyUserPermissions), ps)
		return m.authorizeAndContinueInternal(c, next, ps)
	}
	if apiKey := req.Header.Get(middleware.HeaderApiKey); apiKey != "" {
		return m.apiKeyHeaderAuth(ctx, c, next, apiKey)
	}

	token, fromCookie := extractBearerOrCookieTokenInternal(c)
	if token == "" {
		if m.options.SuccessOptional {
			return next(c)
		}
		return c.JSON(http.StatusUnauthorized, common.APIError{
			Code:    common.APIErrorCodeUnauthorized,
			Message: "Authentication required",
		})
	}

	var authenticatedUser *userdomain.User
	var sessionID string
	var err error
	if fromCookie {
		authenticatedUser, sessionID, err = m.authService.VerifyBrowserToken(ctx, token)
	} else {
		authenticatedUser, sessionID, err = m.authService.VerifyToken(ctx, token)
	}
	if err != nil {
		// A version mismatch means the app self-updated; the session is still valid
		// (the refresh path tolerates the version change and rotates the token), so do
		// NOT clear the cookies. Return a recoverable 401 the frontend refreshes from.
		if errors.Is(err, common.ErrTokenVersionMismatch) {
			return c.JSON(http.StatusUnauthorized, common.APIError{
				Code:    common.APIErrorCodeUnauthorized,
				Message: "Application has been updated. Refreshing session.",
			})
		}

		if errors.Is(err, common.ErrSessionRevoked) || errors.Is(err, common.ErrTokenValidation) {
			cookie.ClearTokenCookie(c.Response(), req)
			return c.JSON(http.StatusUnauthorized, common.APIError{
				Code:    common.APIErrorCodeUnauthorized,
				Message: "Session expired. Please log in again.",
			})
		}

		if m.options.SuccessOptional {
			return next(c)
		}
		return c.JSON(http.StatusUnauthorized, common.APIError{
			Code:    common.APIErrorCodeUnauthorized,
			Message: "Invalid or expired token",
		})
	}

	ps := m.resolvePermissionsOrDeny(ctx, authenticatedUser)
	c.Set(string(middleware.ContextKeyUserID), authenticatedUser.ID)
	c.Set(string(middleware.ContextKeyCurrentUser), authenticatedUser)
	c.Set(string(middleware.ContextKeyCurrentSessionID), sessionID)
	c.SetRequest(req.WithContext(m.authService.ViewerContextForPermissions(req.Context(), sessionID, authenticatedUser.ID, ps)))
	c.Set(string(middleware.ContextKeyUserPermissions), ps)
	return m.authorizeAndContinueInternal(c, next, ps)
}

// apiKeyHeaderAuth authenticates an X-API-Key credential: a user-owned API key
// first, then an environment access token presented through the same header.
func (m *AuthMiddleware) apiKeyHeaderAuth(ctx context.Context, c *echo.Context, next echo.HandlerFunc, apiKey string) error {
	if m.apiKeyValidator != nil {
		authenticatedUser, key, err := m.apiKeyValidator.ValidateApiKeyWithID(ctx, apiKey)
		if err == nil && authenticatedUser != nil {
			// Personal keys inherit the owner's role permissions (same
			// resolution as session auth); scoped keys use their own grants.
			var ps *authz.PermissionSet
			if key.Kind == apikey.ApiKeyKindPersonal {
				ps = m.resolvePermissionsOrDeny(ctx, authenticatedUser)
			} else {
				ps = m.resolveApiKeyPermissionsOrDeny(ctx, key.ID)
			}
			c.Set(string(middleware.ContextKeyUserID), authenticatedUser.ID)
			c.Set(string(middleware.ContextKeyCurrentUser), authenticatedUser)
			c.Set(string(middleware.ContextKeyUserPermissions), ps)
			c.Set(string(middleware.ContextKeyAuthMethod), "api_key")
			c.SetRequest(c.Request().WithContext(m.authService.ViewerContextForPermissions(c.Request().Context(), "", authenticatedUser.ID, ps)))
			return m.authorizeAndContinueInternal(c, next, ps)
		}
	}
	if env, ok := m.resolveEnvironmentAccessToken(ctx, apiKey).Get(); ok {
		ps := environmentScopedInternal(c, env)
		return m.authorizeAndContinueInternal(c, next, ps)
	}
	return c.JSON(http.StatusUnauthorized, common.APIError{
		Code:    common.APIErrorCodeUnauthorized,
		Message: "Invalid or expired API key",
	})
}

func (m *AuthMiddleware) authorizeAndContinueInternal(c *echo.Context, next echo.HandlerFunc, ps *authz.PermissionSet) error {
	if m.options.AdminRequired && !ps.IsGlobalAdmin() {
		if ps != nil && ps.UserID != "" {
			return c.JSON(403, common.APIError{Code: "access.admin-required", Message: "access.admin-required"})
		}
		return c.JSON(http.StatusForbidden, common.APIError{
			Code:    "FORBIDDEN",
			Message: "You don't have permission to access this resource",
		})
	}
	return next(c)
}

// resolvePermissionsOrDeny returns the user's permission set, or an empty
// (deny-all) set if the resolver is unavailable or fails. Resolver failures
// are logged.
func (m *AuthMiddleware) resolvePermissionsOrDeny(ctx context.Context, authenticatedUser *userdomain.User) *authz.PermissionSet {
	if authenticatedUser == nil {
		return authz.NewPermissionSet()
	}
	if m.roleResolver == nil {
		return access.Synthesize(authenticatedUser.ID, nil)
	}
	revision := access.Revision(authenticatedUser.ID)
	ps, err := m.roleResolver.ResolvePermissions(ctx, authenticatedUser.ID)
	if err != nil || ps == nil {
		slog.WarnContext(ctx, "failed to resolve user permissions for Echo auth", "error", err)
		return access.Synthesize(authenticatedUser.ID, nil)
	}
	copy := *ps
	copy.RightsRevision = revision
	return &copy
}

// resolveApiKeyPermissionsOrDeny returns the API key's per-key PermissionSet,
// or an empty (deny-all) set on failure. Falling back to the owner's role
// permissions would defeat per-key scoping, so failures are explicitly denied.
func (m *AuthMiddleware) resolveApiKeyPermissionsOrDeny(ctx context.Context, apiKeyID string) *authz.PermissionSet {
	if m.roleResolver == nil || apiKeyID == "" {
		return authz.NewPermissionSet()
	}
	ps, err := m.roleResolver.ResolveApiKeyPermissions(ctx, apiKeyID)
	if err != nil || ps == nil {
		slog.WarnContext(ctx, "failed to resolve api key permissions for Echo auth", "error", err)
		return authz.NewPermissionSet()
	}
	return ps
}

func (m *AuthMiddleware) resolveEnvironmentAccessToken(ctx context.Context, token string) mo.Option[*environment.Environment] {
	if m.envTokenResolver == nil {
		return mo.None[*environment.Environment]()
	}

	env, err := m.envTokenResolver.ResolveEnvironmentByAccessToken(ctx, token)
	if err != nil || env == nil {
		return mo.None[*environment.Environment]()
	}

	return mo.Some(env)
}

func isPreflightInternal(c *echo.Context) bool {
	return c.Request().Method == http.MethodOptions
}

func agentSudoInternal(c *echo.Context) {
	agentUser := createAgentSudoUserInternal()
	c.Set(string(middleware.ContextKeyUserID), agentUser.ID)
	c.Set(string(middleware.ContextKeyCurrentUser), agentUser)
	c.Set(string(middleware.ContextKeyUserPermissions), authz.SudoPermissionSet())
	c.Set(string(middleware.ContextKeyAuthMethod), "agent_token")
}

func environmentScopedInternal(c *echo.Context, env *environment.Environment) *authz.PermissionSet {
	envUser := &userdomain.User{
		ID:       "environment:" + env.ID,
		Username: env.Name,
	}
	c.Set(string(middleware.ContextKeyUserID), envUser.ID)
	c.Set(string(middleware.ContextKeyCurrentUser), envUser)
	ps := authz.EnvironmentPermissionSet(env.ID)
	c.Set(string(middleware.ContextKeyUserPermissions), ps)
	c.Set(string(middleware.ContextKeyAuthMethod), "environment_access_token")
	return ps
}

func extractBearerOrCookieTokenInternal(c *echo.Context) (string, bool) {
	req := c.Request()
	authHeader := req.Header.Get("Authorization")
	if after, ok := BearerToken(authHeader); ok {
		return after, false
	}
	if tok, err := cookie.GetTokenCookie(req); err == nil && tok != "" {
		return tok, true
	}
	return "", false
}
