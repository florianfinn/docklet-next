// Source: getarcaneapp/arcane/backend/internal/bootstrap/router_bootstrap.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed Svelte-specific metadata or asset paths; functional cut: removed federated credentials; functional cut: removed Swarm; functional cut: removed OIDC and passkeys.; Agent Echo routes omit hub-only ingress, UI and test endpoints; session-bound viewer lifetime and durable validation.

package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strings"

	"github.com/florianfinn/docklet-next/types"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	echomiddleware "github.com/labstack/echo/v5/middleware"
	"github.com/samber/slog-echo/v2"
	"go.uber.org/fx"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/api/ws"
	"github.com/florianfinn/docklet-next/backend/frontend"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	hubbackup "github.com/florianfinn/docklet-next/backend/internal/system/children/backup"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

var (
	registerPlaywrightRoutes []func(apiGroup *echo.Group, deps api.HandlerDeps)
	registerBuildableRoutes  []func(apiGroup *echo.Group, deps api.HandlerDeps)
)

var loggerSkipPatterns = []string{
	"POST /api/tunnel/poll",
	"GET /api/environments/*/ws/containers/*/logs",
	"GET /api/environments/*/ws/containers/*/stats",
	"GET /api/environments/*/ws/containers/*/terminal",
	"GET /api/environments/*/ws/projects/*/logs",
	"GET /api/environments/*/ws/system/stats",
	"GET /api/stream",
	"GET /assets/*",
	"GET /img",
	"GET /api/health",
	"HEAD /api/health",
	"GET /api/app-images/*",
	"PUT /api/environments/*/uploads/*/*/chunks/*",
}

func shouldLogRequestInternal(c *echo.Context, _ error) bool {
	mp := c.Request().Method + " " + c.Request().URL.Path
	for _, pat := range loggerSkipPatterns {
		if pat == mp {
			return false
		}
		if before, ok := strings.CutSuffix(pat, "/*"); ok {
			if strings.HasPrefix(mp, before) {
				return false
			}
		}
		if ok, _ := path.Match(pat, mp); ok {
			return false
		}
		if strings.HasSuffix(pat, "/") && strings.HasPrefix(mp, pat) {
			return false
		}
	}
	return true
}

// requestLoggerMiddlewareInternal wraps slog-echo and filters out internal
// edge tunnel requests plus high-volume endpoints (health, WS, static).
func requestLoggerMiddlewareInternal() echo.MiddlewareFunc {
	loggerMiddleware := slogecho.NewWithConfig(slog.Default(), slogecho.Config{
		DefaultLevel:     slog.LevelInfo,
		ClientErrorLevel: slog.LevelWarn,
		ServerErrorLevel: slog.LevelError,
		Filters:          []slogecho.Filter{shouldLogRequestInternal},
		WithRequestID:    true,
	})

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if edge.IsInternalTunnelRequest(c.Request().Context()) {
				return next(c)
			}
			if c.Request().Body == nil {
				c.Request().Body = http.NoBody
			}
			return loggerMiddleware(next)(c)
		}
	}
}

func createAuthValidatorInternal(deps api.HandlerDeps) middleware.AuthValidator {
	resolveUser := func(ctx context.Context, user *usertypes.Actor) *authz.PermissionSet {
		ps, err := deps.Role.Service().ResolvePermissions(ctx, user.ID)
		if err != nil || ps == nil {
			slog.WarnContext(ctx, "failed to resolve user permissions for env proxy", "error", err)
			return authz.NewPermissionSet()
		}
		return ps
	}
	resolveKey := func(ctx context.Context, keyID string) *authz.PermissionSet {
		ps, err := deps.Role.Service().ResolveApiKeyPermissions(ctx, keyID)
		if err != nil || ps == nil {
			slog.WarnContext(ctx, "failed to resolve api key permissions for env proxy", "error", err)
			return authz.NewPermissionSet()
		}
		return ps
	}
	return func(ctx context.Context, c *echo.Context) (*authz.PermissionSet, *usertypes.Actor, bool) {
		req := c.Request()
		if deps.Auth.Service().SignInMode() != "local" {
			if apiKey := req.Header.Get(middleware.HeaderApiKey); apiKey != "" {
				if envID, err := deps.ApiKey.Service().GetEnvironmentByApiKey(ctx, apiKey); err == nil && envID != nil {
					return authz.EnvironmentPermissionSet(*envID), nil, true
				}
			}
			actor, ok := verifyRouterRequestUserInternal(ctx, deps.Auth.Service(), req)
			return authz.SudoPermissionSet(), actor, ok
		}
		// Check for API key authentication
		if apiKey := req.Header.Get(middleware.HeaderApiKey); apiKey != "" {
			// User-owned API key: personal keys inherit the owner's role
			// permissions; scoped keys are limited to their own grants.
			if user, key, err := deps.ApiKey.Service().ValidateApiKeyWithID(ctx, apiKey); err == nil && user != nil {
				if key != nil && key.Kind != apikey.ApiKeyKindPersonal {
					return resolveKey(ctx, key.ID), user.Actor(), true
				}
				return resolveUser(ctx, user.Actor()), user.Actor(), true
			}
			// Environment bootstrap key (user_id = NULL): used by the proxy when forwarding
			// requests to a remote env whose apiUrl resolves back to this manager.
			if envID, err := deps.ApiKey.Service().GetEnvironmentByApiKey(ctx, apiKey); err == nil && envID != nil {
				return authz.EnvironmentPermissionSet(*envID), nil, true
			}
			return nil, nil, false
		}

		user, ok := verifyRouterRequestUserInternal(ctx, deps.Auth.Service(), req)
		if !ok {
			return nil, nil, false
		}
		return resolveUser(ctx, user), user, true
	}
}

func verifyRouterRequestUserInternal(ctx context.Context, authService *auth.AuthService, req *http.Request) (*usertypes.Actor, bool) {
	if authService.SignInMode() != "local" {
		requestUser, err := authService.ModeRequestUser(ctx, req)
		return requestUser.Actor(), err == nil && requestUser != nil
	}
	if token, ok := auth.BearerToken(req.Header.Get("Authorization")); ok {
		user, sessionID, err := authService.VerifyToken(ctx, token)
		if err == nil && user != nil {
			*req = *req.WithContext(authService.ViewerContext(ctx, sessionID, user.ID))
		}
		return user.Actor(), err == nil && user != nil
	}

	browserToken, err := cookie.GetTokenCookie(req)
	if err != nil || browserToken == "" {
		return nil, false
	}
	user, sessionID, err := authService.VerifyBrowserToken(ctx, browserToken)
	if err == nil && user != nil {
		*req = *req.WithContext(authService.ViewerContext(ctx, sessionID, user.ID))
	}
	return user.Actor(), err == nil && user != nil
}

type RouterParams struct {
	fx.In

	Context        context.Context
	Lifecycle      fx.Lifecycle
	Config         *config.Config
	HandlerDeps    api.HandlerDeps
	AuthMiddleware *auth.AuthMiddleware
	TunnelRegistry *edge.TunnelRegistry
}

// newEchoInternal builds the Echo instance with path parameter values
// percent-decoded before they reach handlers (RFC 3986 §6.2.2.2); route
// matching still happens on the escaped path, so an encoded "/" cannot
// change which route matches.
func newEchoInternal() *echo.Echo {
	return echo.NewWithConfig(echo.Config{
		Router: echo.NewRouter(echo.RouterConfig{
			AllowOverwritingRoute:   true,
			UnescapePathParamValues: true,
		}),
	})
}

func newRouter(p RouterParams) (*echo.Echo, *edge.TunnelServer) {
	ctx := p.Context
	cfg := p.Config
	deps := p.HandlerDeps
	hubMode := !cfg.AgentMode && !cfg.EdgeAgent

	e := newEchoInternal()

	trustedProxyNets := parseTrustedProxyCIDRsInternal(cfg.TrustedProxies)
	if cfg.TrustedProxies != "" && len(trustedProxyNets) == 0 {
		slog.WarnContext(ctx, "TRUSTED_PROXIES set but no valid CIDRs found; falling back to direct IP extraction")
	}
	if len(trustedProxyNets) == 0 {
		e.IPExtractor = echo.ExtractIPDirect()
	} else {
		opts := make([]echo.TrustOption, 0, len(trustedProxyNets))
		for _, ipnet := range trustedProxyNets {
			opts = append(opts, echo.TrustIPRange(ipnet))
		}
		e.IPExtractor = echo.ExtractIPFromXFFHeader(opts...)
	}

	e.Use(echomiddleware.Recover())
	e.Use(echomiddleware.RequestID())
	e.Use(activity.RuntimeReceiverMiddleware(cfg.AgentMode || cfg.EdgeAgent, activity.RuntimeAdmissionLimit))
	e.Use(requestLoggerMiddlewareInternal())
	e.Use(secureCookieContextMiddlewareInternal(trustedProxyNets))
	if hubMode && deps.System != nil {
		if gate := deps.System.Service().BackupMaintenanceGate(); gate != nil {
			e.Use(maintenanceMiddlewareInternal(gate))
			if p.TunnelRegistry != nil {
				gate.OnBegin(p.TunnelRegistry.CloseActiveForMaintenance)
			}
		}
	}

	authMiddleware := p.AuthMiddleware
	e.Use(middleware.NewCORSMiddleware(cfg).Add())
	e.Use(middleware.NewCSRFMiddleware(cfg).
		WithTransportValidator(csrfTransportValidatorInternal(deps)).
		WithLocalCredentialValidator(authMiddleware.ValidateLocalHeaderCredential).Add())

	apiGroup := e.Group("/api")

	apiGroup.Use(middleware.PerIPRateLimitForPaths(
		[]string{
			"/api/auth/login",
			"/api/auth/refresh",
		}, 5, 5,
	))
	// Keyed per token for stored tokens, per source IP for everything else.
	apiGroup.Use(middleware.PerTokenRateLimitForPaths(
		[]string{"/api/webhooks/trigger/:token"}, 60, 10, deps.Webhook.Service().IsKnownToken,
	))
	// Only POST agent ingestion needs this brute-force ceiling outside session auth.
	// Authenticated GET pagination consumes no ingestion budget. Other registrations:
	// login/refresh per IP, webhook per token/IP, and edge_bootstrap.go per-route
	// enroll POST and connect GET, each with separate IP and agent-token limits.
	eventIngestionLimit := middleware.PerIPRateLimitForPaths([]string{"/api/events"}, 60, 30)
	apiGroup.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		limited := eventIngestionLimit(next)
		return func(c *echo.Context) error {
			if c.Request().Method != http.MethodPost {
				return next(c)
			}
			return limited(c)
		}
	})
	handlerAppCtx := handlerutil.NewActivityAppContext(ctx)

	envResolver := func(ctx context.Context, id string) (string, *string, bool, error) {
		env, err := deps.Environment.Service().GetEnvironmentByIDCached(ctx, id)
		if err != nil || env == nil {
			return "", nil, false, err
		}
		return env.ApiUrl, env.AccessToken, env.Enabled, nil
	}

	if hubMode {
		// These token ingress endpoints belong to the hub only.
		api.RegisterWebhookTrigger(apiGroup, deps.Webhook.Service(), handlerAppCtx)
		deps.Event.RegisterAgentRoutes(apiGroup, func(ctx context.Context, token string) (string, error) {
			env, err := deps.Environment.Service().ResolveEnvironmentByAccessToken(ctx, token)
			if err != nil {
				return "", err
			}
			if env == nil || !env.Enabled || env.ID == "0" {
				return "", nil
			}
			return env.ID, nil
		})
	}

	permissionMatcher := authz.NewPermissionMatcher()

	var inventorySnapshots *hoststate.Store
	if hubMode {
		inventorySnapshots = deps.Environment.Service().ReadSnapshots()
	}
	envProxyMiddleware := middleware.NewEnvProxyMiddlewareWithParamAndRegistry(
		types.LocalDockerEnvironmentID,
		"id",
		envResolver,
		createAuthValidatorInternal(deps),
		permissionMatcher,
		p.TunnelRegistry,
		middleware.WebSocketOriginValidator(cfg, func(req *http.Request) bool {
			return authMiddleware.ValidateNonCookieCredential(req) || csrfTransportValidatorInternal(deps)(req)
		}),
		middleware.EnvironmentProxySecurity{PointAccess: func(ctx context.Context, env, target, point string) error {
			return deps.Updater.Service().CheckRestorePointAccess(ctx, env, target, point, deps.Environment.Service())
		}, PointHolds: deps.Activity.Service().ReadPointHolds, Snapshots: inventorySnapshots, ProxyIdentityHeader: cfg.ProxyIdentityHeader(), AgentAccess: func(ctx context.Context, id string, token *string, method, path string) (context.Context, context.CancelFunc, error) {
			return deps.Environment.Service().AgentAccess(ctx, id, token, method, path)
		}, InventoryPending: deps.Environment.Service().InventoryPending},
	)
	apiGroup.Use(envProxyMiddleware)

	humaAPI := api.SetupAPI(e, apiGroup, handlerAppCtx, cfg, deps)

	// Populate the proxy permission matcher from the registered API surface so
	// remote-environment requests are authorized with the same permissions
	// enforced locally. The matcher pointer is shared with the proxy middleware
	// constructed above; population completes here, before the server serves traffic.
	permissionMatcher.CollectFromHumaAPI(humaAPI)
	ws.AddProxiedPermissions(permissionMatcher)

	if hubMode {
		for _, register := range registerBuildableRoutes {
			register(apiGroup, deps)
		}
	}

	// Remaining echo handlers (WebSocket/streaming)
	ws.NewWebSocketHandler(apiGroup, deps.Project.Service(), deps.Container.Service(), deps.System.Service(), deps.Diagnostics, authMiddleware, cfg)

	// Register edge tunnel endpoint for manager to accept agent connections
	// This is only registered when NOT in agent mode (i.e., running as manager)
	var tunnelServer *edge.TunnelServer
	if hubMode {
		tunnelServer = registerEdgeTunnelRoutes(ctx, p.Lifecycle, cfg, apiGroup, deps.Environment.Service(), deps.Event.Service(), deps.Notification.Service(), p.TunnelRegistry)
	}

	if hubMode && cfg.Environment != "production" {
		for _, registerFunc := range registerPlaywrightRoutes {
			registerFunc(apiGroup, deps)
		}
	}

	apiNotFound := func(c *echo.Context) error {
		return c.JSON(http.StatusNotFound, map[string]any{
			"success": false,
			"error":   "API endpoint not found: " + c.Request().URL.Path,
		})
	}
	apiGroup.RouteNotFound("", apiNotFound)
	apiGroup.RouteNotFound("/*", apiNotFound)

	if hubMode {
		//nolint:staticcheck,nolintlint // SA4023 only under exclude_frontend: the stub always returns ErrFrontendNotIncluded
		if err := frontend.RegisterFrontend(e); err != nil {
			if errors.Is(err, frontend.ErrFrontendNotIncluded) {
				slog.DebugContext(ctx, "Frontend not included in this build; skipping frontend registration")
			} else {
				slog.ErrorContext(ctx, "Failed to register frontend", "error", err)
			}
		}
	}

	return e, tunnelServer
}

func maintenanceMiddlewareInternal(gate *hubbackup.MaintenanceGate) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			req := c.Request()
			status := gate.Status()
			if status.Active {
				return maintenanceReadOrErrorInternal(c, status)
			}
			// Keep accepted snapshot/restore responses alive until their workers drain them.
			if req.Method == http.MethodPost && (req.URL.Path == "/api/backups" || (strings.HasPrefix(req.URL.Path, "/api/backups/") && strings.HasSuffix(req.URL.Path, "/restore"))) {
				release, err := gate.Admit()
				if err != nil {
					return maintenanceReadOrErrorInternal(c, gate.Status())
				}
				defer release()
				return next(c)
			}
			ctx, cancel := context.WithCancel(req.Context())
			release, err := gate.AdmitCancel(cancel)
			if err != nil {
				cancel()
				return maintenanceReadOrErrorInternal(c, gate.Status())
			}
			defer release()
			defer cancel()
			c.SetRequest(req.WithContext(ctx))
			return next(c)
		}
	}
}

func maintenanceReadOrErrorInternal(c *echo.Context, status backuptypes.SystemBackupMaintenance) error {
	req := c.Request()
	if req.Method == http.MethodGet && req.URL.Path == "/api/backups/maintenance" {
		c.Response().Header().Set("Cache-Control", "no-store")
		return c.JSON(http.StatusOK, status)
	}
	if req.URL.Path == "/api/health" && (req.Method == http.MethodGet || req.Method == http.MethodHead) {
		c.Response().Header().Set("Cache-Control", "no-store")
		if req.Method == http.MethodHead {
			return c.NoContent(http.StatusServiceUnavailable)
		}
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "DOWN"})
	}
	c.Response().Header().Set("Content-Type", "application/problem+json")
	return c.JSON(http.StatusServiceUnavailable, maintenanceProblemInternal())
}

func maintenanceProblemInternal() map[string]any {
	return map[string]any{
		"title": "Service Unavailable", "status": http.StatusServiceUnavailable,
		"detail": "backup.maintenance",
	}
}

// parseTrustedProxyCIDRsInternal parses TRUSTED_PROXIES into a list of
// validated networks. Invalid entries are logged and skipped.
func parseTrustedProxyCIDRsInternal(raw string) []*net.IPNet {
	if raw == "" {
		return nil
	}
	var nets []*net.IPNet
	for cidr := range strings.SplitSeq(raw, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			slog.WarnContext(context.Background(), "invalid TRUSTED_PROXIES CIDR, ignoring", "cidr", cidr, "error", err) //nolint:forbidigo // Startup configuration parsing has no request context.
			continue
		}
		nets = append(nets, ipnet)
	}
	return nets
}

// secureCookieContextMiddlewareInternal records whether the request should be
// treated as HTTPS for cookie-emitting handlers. X-Forwarded-Proto is honored
// ONLY when the direct TCP peer is in TRUSTED_PROXIES — an untrusted client
// setting the header directly cannot trick the server into issuing Secure /
// __Host- cookies over plain HTTP.
func secureCookieContextMiddlewareInternal(trustedProxyNets []*net.IPNet) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			req := c.Request()
			secure := req.TLS != nil
			if !secure && len(trustedProxyNets) > 0 &&
				strings.EqualFold(req.Header.Get("X-Forwarded-Proto"), "https") &&
				remoteAddrInTrustedProxiesInternal(req.RemoteAddr, trustedProxyNets) {
				secure = true
			}
			if secure {
				c.SetRequest(req.WithContext(context.WithValue(req.Context(), cookie.SecureCookieContextKey{}, true)))
			}
			return next(c)
		}
	}
}

// remoteAddrInTrustedProxiesInternal reports whether the direct TCP peer
// address of the request falls within any of the configured trusted-proxy
// networks. Unparseable remote addresses are treated as untrusted.
func remoteAddrInTrustedProxiesInternal(remoteAddr string, nets []*net.IPNet) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
