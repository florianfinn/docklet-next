// Source: getarcaneapp/arcane/backend/internal/middleware/environment.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed GitOps, saved Git repositories and Swarm; validate original WebSocket client origin before forwarding credentials.

package middleware

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/types/container"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/labstack/echo/v5"
	"github.com/samber/mo"
	"go.getarcane.app/kit/pkg"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/httpx"
)

const (
	apiEnvironmentsPrefix  = "/api/environments/"
	environmentsPathMarker = "/environments/"

	// proxyTimeout is intentionally generous because some proxied operations
	// (e.g., image pulls with progress streaming) can take multiple minutes.
	proxyTimeout = 30 * time.Minute

	maxProxiedWorkspaceManifestBytes = 1024 * 1024
)

// managementEndpointSet contains paths handled locally and never proxied to remote environments.
var managementEndpointSet = map[string]struct{}{
	"/test":            {},
	"/heartbeat":       {},
	"/sync-registries": {},
	"/sync":            {},
	"/deployment":      {},
	"/agent/pair":      {},
	"/version":         {},
	"/settings":        {},
	"/job-schedules":   {},
	"/jobs":            {},
}

// EnvResolver resolves an environment ID to its connection details.
// Returns: apiURL, accessToken, enabled, error
type EnvResolver func(ctx context.Context, id string) (string, *string, bool, error)

// AuthValidator validates authentication for a request and resolves the
// caller's effective permission set. The boolean result reports whether the
// request is authenticated; the permission set is used to authorize proxied
// requests against the target environment. Sudo permission sets (internal
// agent proxies) bypass authorization. The resolved user is returned so
// per-user context (e.g. the icon catalog preference) can travel with the
// proxied request; it is nil for callers that are not a user (environment
// bootstrap keys).
type AuthValidator func(ctx context.Context, c *echo.Context) (*authz.PermissionSet, *usertypes.Actor, bool)

// EnvironmentMiddleware proxies requests for remote environments to their respective agents.
type EnvironmentMiddleware struct {
	localID       string
	paramName     string
	resolver      EnvResolver
	authValidator AuthValidator
	httpClient    *http.Client
	registry      *edge.TunnelRegistry
	matcher       *authz.PermissionMatcher
	// checkOrigin shares the local WebSocket origin and credential policy.
	checkOrigin         func(*http.Request) bool
	proxyIdentityHeader string
	snapshots           *hoststate.Store
	inventoryPending    func(context.Context, string) (bool, error)
	agentAccess         remenv.AgentAccessCheck
}

// NewEnvProxyMiddlewareWithParamAndRegistry creates middleware with an injected tunnel registry.
func NewEnvProxyMiddlewareWithParamAndRegistry(
	localID,
	paramName string,
	resolver EnvResolver,
	authValidator AuthValidator,
	matcher *authz.PermissionMatcher,
	registry *edge.TunnelRegistry,
	checkOrigin func(*http.Request) bool,
	options ...EnvironmentProxySecurity,
) echo.MiddlewareFunc {
	if registry == nil {
		registry = edge.NewTunnelRegistry()
	}

	m := &EnvironmentMiddleware{
		localID:       localID,
		paramName:     paramName,
		resolver:      resolver,
		authValidator: authValidator,
		httpClient:    &http.Client{Timeout: proxyTimeout},
		registry:      registry,
		matcher:       matcher,
		checkOrigin:   checkOrigin,
	}
	if len(options) > 0 {
		m.proxyIdentityHeader = options[0].ProxyIdentityHeader
		m.agentAccess = options[0].AgentAccess
		m.snapshots = options[0].Snapshots
		m.inventoryPending = options[0].InventoryPending
		if options[0].HTTPClient != nil {
			m.httpClient = options[0].HTTPClient
		}
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			return m.Handle(c, next)
		}
	}
}

// Handle is the main middleware handler.
func (m *EnvironmentMiddleware) Handle(c *echo.Context, next echo.HandlerFunc) error {
	envID := m.extractEnvironmentID(c)

	if envID == "" || envID == m.localID {
		return next(c)
	}

	if !m.hasResourcePath(c, envID) {
		return next(c)
	}

	// SECURITY: Validate authentication BEFORE proxying to remote environments.
	var perms *authz.PermissionSet
	var user *usertypes.Actor
	if m.authValidator != nil {
		ps, u, ok := m.authValidator(c.Request().Context(), c)
		if !ok {
			return c.JSON(http.StatusUnauthorized, map[string]any{
				"success": false,
				"data":    map[string]any{"error": "Authentication required to access remote environments"},
			})
		}
		perms, user = ps, u
	}
	// Check client authority before Edge forwarding injects server-side agent tokens.
	if httpx.IsWebSocketUpgradeRequest(c.Request()) && (m.checkOrigin == nil || !m.checkOrigin(c.Request())) {
		return echo.NewHTTPError(http.StatusForbidden, "websocket origin not allowed")
	}

	m.setIconCatalogHeaderInternal(c, user)
	m.setUpdateInitiatorHeadersInternal(c, user)

	apiURL, accessToken, enabled, err := m.resolver(c.Request().Context(), envID)
	if err != nil || apiURL == "" {
		return c.JSON(http.StatusNotFound, map[string]any{
			"success": false,
			"data":    map[string]any{"error": "Environment not found"},
		})
	}

	// SECURITY: Enforce the caller's per-environment permission BEFORE proxying.
	// Remote agents run with a sudo permission set and perform no authorization
	// of their own, so authorization for proxied requests must happen here,
	// mirroring the per-operation RequirePermission checks used for the local
	// environment.
	if m.proxyPermissionDenied(c, perms, envID) {
		return c.JSON(http.StatusForbidden, map[string]any{
			"success": false,
			"data":    map[string]any{"error": "You don't have permission to perform this action on this environment"},
		})
	}

	if err := InlineImageCredentials(c.Request().URL.Path, c.Request().Header, c.QueryParams()); err != nil {
		problem := imageDTO.Neutral(err, "registry.inline-credentials-not-allowed", 400)
		return c.JSON(problem.Status, problem)
	}
	if err := checkImageUpgrade(c); err != nil {
		problem := imageDTO.Neutral(err, "update.invalid-target", 400)
		return c.JSON(problem.Status, problem)
	}
	return m.inventoryRead(c, envID, func() error {
		if !enabled {
			return c.JSON(http.StatusBadRequest, map[string]any{"success": false, "code": "agent.unavailable"})
		}
		return m.forwardInventoryRequest(c, envID, apiURL, accessToken)
	})
}

func (m *EnvironmentMiddleware) hasActiveEdgeTunnelInternal(envID string) bool {
	_, ok := m.getActiveEdgeTunnelInternal(envID).Get()
	return ok
}

func (m *EnvironmentMiddleware) forwardInventoryRequest(c *echo.Context, envID, apiURL string, accessToken *string) error {

	suffix := m.buildResourceSuffix(c.Request().URL.Path, envID)
	// Pending credentials retain snapshots without probing an unpaired agent.
	inventory := m.snapshots != nil && c.Request().Method == http.MethodGet && inventorySuffix(suffix, strings.Split(strings.Trim(suffix, "/"), "/"))
	if inventory && m.inventoryPending != nil {
		pending, err := m.inventoryPending(c.Request().Context(), envID)
		if err != nil || pending {
			return m.abortEdgeTunnelUnavailable(c)
		}
	}
	// Cached inventory answers immediately; cold reads allow bounded tunnel recovery.
	edgeInventory := inventory && isEdgeEnvironmentURLInternal(apiURL)
	if edgeInventory && !m.hasActiveEdgeTunnelInternal(envID) && !m.recoverInventoryTunnel(c, envID, suffix) {
		return m.abortEdgeTunnelUnavailable(c)
	}
	if m.agentAccess != nil {
		ctx, cancel, err := m.agentAccess(c.Request().Context(), envID, accessToken, c.Request().Method, m.buildProxyPath(c, envID))
		if err != nil {
			status, code := remenv.AgentAccessFailure(err)
			return c.JSON(status, map[string]any{"success": false, "code": code})
		}
		defer cancel()
		c.SetRequest(c.Request().WithContext(ctx))
	}
	isEdgeEnvironment := isEdgeEnvironmentURLInternal(apiURL)

	if handled, proxyActiveEdgeTunnelErr := m.proxyActiveEdgeTunnelInternal(c, envID, accessToken); handled {
		return proxyActiveEdgeTunnelErr
	}

	if isEdgeEnvironment {
		if !edgeInventory {
			if handled, proxyRecoveredEdgeTunnelErr := m.proxyRecoveredEdgeTunnelInternal(c, envID, accessToken); handled {
				return proxyRecoveredEdgeTunnelErr
			}
		}

		slog.WarnContext(c.Request().Context(), "No active edge tunnel for environment", "environmentId", envID)
		return m.abortEdgeTunnelUnavailable(c)
	}

	target := m.buildTargetURL(c, envID, apiURL)

	if httpx.IsWebSocketUpgradeRequest(c.Request()) {
		return m.proxyWebSocket(c, target, accessToken, envID)
	}
	return m.proxyHTTP(c, target, accessToken)
}

// setIconCatalogHeaderInternal stamps the authenticated user's icon catalog
// preference onto the outgoing proxied request. Remote agents authenticate the
// proxy as a synthetic user with no preferences, so without this every remote
// environment resolves icons against the default catalog.
//
// SECURITY: the header is always cleared first, so a browser-supplied value
// never rides through; only the server-resolved preference is forwarded.
func (m *EnvironmentMiddleware) setIconCatalogHeaderInternal(c *echo.Context, user *usertypes.Actor) {
	c.Request().Header.Del(HeaderIconCatalog)
	if user == nil || user.Preferences.IconCatalog == nil || *user.Preferences.IconCatalog == "" {
		return
	}
	c.Request().Header.Set(HeaderIconCatalog, *user.Preferences.IconCatalog)
}

func (m *EnvironmentMiddleware) setUpdateInitiatorHeadersInternal(c *echo.Context, user *usertypes.Actor) {
	headers := c.Request().Header
	headers.Del(HeaderUpdateInitiatorID)
	headers.Del(HeaderUpdateInitiatorName)
	headers.Del(HeaderUpdateInitiatorDisplayName)
	updatePath := strings.Contains(c.Request().URL.Path, "/containers/") && strings.HasSuffix(c.Request().URL.Path, "/update")
	if c.Request().Method != http.MethodPost || !updatePath {
		return
	}
	if user == nil || user.ID == "" {
		return
	}
	headers.Set(HeaderUpdateInitiatorID, user.ID)
	headers.Set(HeaderUpdateInitiatorName, user.Username)
	if user.DisplayName != nil {
		headers.Set(HeaderUpdateInitiatorDisplayName, *user.DisplayName)
	}
}

// proxyPermissionDenied reports whether the caller lacks permission to perform
// the proxied request against environment envID. It mirrors the per-operation
// RequirePermission checks enforced for the local environment: the permission
// required for the (method, path) is looked up in the matcher and evaluated
// against the caller's permission set for the target environment.
//
// Sudo callers bypass the check. Requests whose (method, path) has no known
// permission mapping are denied (default-deny), so a newly added proxied route
// cannot silently bypass authorization before it is mapped.
func (m *EnvironmentMiddleware) proxyPermissionDenied(c *echo.Context, ps *authz.PermissionSet, envID string) bool {
	if m.matcher == nil {
		return false
	}
	if ps != nil && ps.Sudo {
		return false
	}

	method := c.Request().Method
	suffix := m.buildResourceSuffix(c.Request().URL.Path, envID)
	if kind, ok := uploadSessionKindInternal(method, suffix); ok {
		// Upload-session routes derive their permission from the {kind} path
		// segment, so they carry no static mapping in the matcher; resolve the
		// kind here and fail closed on unknown kinds.
		perm, known := authz.UploadKindPermission(kind)
		if !known || !ps.Allows(perm, envID) {
			slog.DebugContext(c.Request().Context(), "Denying proxied upload session request: permission denied",
				"method", method, "path", suffix, "kind", kind, "environmentId", envID)
			return true
		}
		return false
	}
	perm, ok := m.matcher.Lookup(method, suffix).Get()
	if !ok {
		slog.WarnContext(c.Request().Context(), "Denying proxied request with no known permission mapping",
			"method", method, "path", suffix, "environmentId", envID)
		return true
	}
	if perm == "" {
		// Explicitly public proxied route (e.g. public settings): allowed for
		// any authenticated caller, matching local enforcement.
		return false
	}

	scopeEnvID := kit.Ternary(authz.IsEnvScoped(perm), envID, "")
	if !ps.Allows(perm, scopeEnvID) {
		slog.DebugContext(c.Request().Context(), "Denying proxied request: permission denied",
			"method", method, "path", suffix, "permission", perm, "environmentId", envID)
		return true
	}
	var required []string
	var err error
	if isVolumeWorkspaceUpdateRequestInternal(method, suffix) {
		required, err = proxiedVolumeWorkspacePermissionsInternal(c.Request())
	}
	if err != nil {
		slog.DebugContext(c.Request().Context(), "Denying proxied request with invalid body",
			"path", suffix, "environmentId", envID, "error", err)
		return true
	}
	required = withContainerResourceSortPermissionsInternal(required, c.Request(), method, suffix)
	for _, operationPermission := range required {
		if !ps.Allows(operationPermission, envID) {
			slog.DebugContext(c.Request().Context(), "Denying proxied request: body-derived permission denied",
				"path", suffix, "permission", operationPermission, "environmentId", envID)
			return true
		}
	}
	return false
}

// uploadSessionKindInternal extracts the upload kind from the exact
// upload-session route shapes: POST /uploads/{kind}, GET or DELETE
// /uploads/{kind}/{uploadId}, and PUT /uploads/{kind}/{uploadId}/chunks/{n}.
func uploadSessionKindInternal(method, suffix string) (string, bool) {
	segments := strings.Split(strings.Trim(suffix, "/"), "/")
	if len(segments) < 2 || segments[0] != "uploads" || segments[1] == "" {
		return "", false
	}
	switch {
	case len(segments) == 2 && method == http.MethodPost:
		return segments[1], true
	case len(segments) == 3 && (method == http.MethodGet || method == http.MethodDelete):
		return segments[1], true
	case len(segments) == 5 && method == http.MethodPut && segments[3] == "chunks":
		return segments[1], true
	}
	return "", false
}

func isVolumeWorkspaceUpdateRequestInternal(method, suffix string) bool {
	if method != http.MethodPut {
		return false
	}
	segments := strings.Split(strings.Trim(suffix, "/"), "/")
	return len(segments) == 3 && segments[0] == "volumes" && segments[1] != "" && segments[2] == "workspace"
}

func proxiedVolumeWorkspacePermissionsInternal(request *http.Request) ([]string, error) {
	if request == nil || request.Body == nil {
		return nil, errors.New("missing multipart request body")
	}
	mediaType, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		return nil, errors.New("invalid multipart content type")
	}

	originalBody := request.Body
	// System temp scratch for the multipart replay buffer: no acfs root exists for it.
	captured, err := os.CreateTemp("", "arcane-volume-workspace-manifest-*")
	if err != nil {
		return nil, fmt.Errorf("create workspace manifest buffer: %w", err)
	}
	defer func() {
		_, _ = captured.Seek(0, io.SeekStart)
		request.Body = &proxiedReplayBodyInternal{
			Reader:   io.MultiReader(captured, originalBody),
			captured: captured,
			original: originalBody,
			path:     captured.Name(),
		}
	}()

	reader := multipart.NewReader(io.TeeReader(originalBody, captured), params["boundary"])
	for {
		part, nextErr := reader.NextPart()
		if nextErr != nil {
			return nil, fmt.Errorf("find workspace manifest part: %w", nextErr)
		}
		if part.FormName() != "manifest" || part.FileName() != "" {
			if closeErr := part.Close(); closeErr != nil {
				return nil, fmt.Errorf("skip multipart field before workspace manifest: %w", closeErr)
			}
			continue
		}
		manifestJSON, readErr := io.ReadAll(io.LimitReader(part, maxProxiedWorkspaceManifestBytes+1))
		closeErr := part.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read workspace manifest: %w", readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close workspace manifest part: %w", closeErr)
		}
		if len(manifestJSON) > maxProxiedWorkspaceManifestBytes {
			return nil, errors.New("workspace manifest is too large")
		}
		var manifest volume.WorkspaceUpdateManifest
		if unmarshalErr := json.Unmarshal(manifestJSON, &manifest); unmarshalErr != nil {
			return nil, fmt.Errorf("decode workspace manifest: %w", unmarshalErr)
		}
		required, valid := authz.VolumeWorkspaceRequiredPermissions(manifest.FileChanges)
		if !valid {
			return nil, errors.New("workspace manifest contains an unknown operation")
		}
		return required, nil
	}
}

// withContainerResourceSortPermissionsInternal adds containers:read for
// resource-sorted container lists, which collect per-container stats.
func withContainerResourceSortPermissionsInternal(required []string, request *http.Request, method, suffix string) []string {
	if request == nil || method != http.MethodGet || strings.Trim(suffix, "/") != "containers" {
		return required
	}
	if container.IsResourceSort(request.URL.Query().Get("sort")) {
		return append(required, authz.PermContainersRead)
	}
	return required
}

type proxiedReplayBodyInternal struct {
	io.Reader

	captured *os.File
	original io.ReadCloser
	path     string
}

func (b *proxiedReplayBodyInternal) Close() error {
	capturedErr := b.captured.Close()
	originalErr := b.original.Close()
	// System temp scratch: no acfs root exists for it.
	removeErr := os.Remove(b.path)
	return errors.Join(capturedErr, originalErr, removeErr)
}

func (m *EnvironmentMiddleware) proxyActiveEdgeTunnelInternal(c *echo.Context, envID string, accessToken *string) (bool, error) {
	tunnel, ok := m.getActiveEdgeTunnelInternal(envID).Get()
	if !ok {
		return false, nil
	}

	slog.DebugContext(c.Request().Context(), "Routing request through edge tunnel", "environmentId", envID, "path", c.Request().URL.Path)
	if err := edge.RequireSessionKey(c.Request().Context(), tunnel); err != nil {
		return true, m.abortEdgeTunnelUnavailable(c)
	}
	m.setProxyContextHeadersInternal(c, accessToken)
	return true, m.proxyThroughTunnelInternal(c, tunnel, envID)
}

func (m *EnvironmentMiddleware) proxyRecoveredEdgeTunnelInternal(c *echo.Context, envID string, accessToken *string) (bool, error) {
	edge.TouchTunnelDemand(envID, edge.DefaultTunnelDemandTTL)

	tunnel, ok := m.waitForActiveEdgeTunnelInternal(c.Request().Context(), envID, edge.DefaultTunnelAcquireTimeout()).Get()
	if !ok {
		return false, nil
	}

	slog.InfoContext(c.Request().Context(), "Recovered edge tunnel during request", "environmentId", envID)
	if err := edge.RequireSessionKey(c.Request().Context(), tunnel); err != nil {
		return true, m.abortEdgeTunnelUnavailable(c)
	}
	m.setProxyContextHeadersInternal(c, accessToken)
	return true, m.proxyThroughTunnelInternal(c, tunnel, envID)
}

func (m *EnvironmentMiddleware) setProxyContextHeadersInternal(c *echo.Context, accessToken *string) {
	remenv.StripHubCredentials(c.Request().Header, m.proxyIdentityHeader)
	if accessToken != nil && *accessToken != "" {
		c.Request().Header.Set(edge.HeaderAgentToken, *accessToken)
		c.Request().Header.Set(edge.HeaderAPIKey, *accessToken)
	}
}

func (m *EnvironmentMiddleware) proxyThroughTunnelInternal(c *echo.Context, tunnel *edge.AgentTunnel, envID string) error {
	proxyPath := m.buildProxyPath(c, envID)
	if httpx.IsWebSocketUpgradeRequest(c.Request()) {
		return edge.ProxyWebSocketRequest(c, tunnel, proxyPath, m.checkOrigin)
	}
	return edge.ProxyHTTPRequest(c, tunnel, proxyPath)
}

// hasResourcePath reports whether the request targets a proxiable resource path.
func (m *EnvironmentMiddleware) hasResourcePath(c *echo.Context, envID string) bool {
	suffix, ok := strings.CutPrefix(c.Request().URL.Path, apiEnvironmentsPrefix+envID)
	if !ok || len(suffix) <= 1 || suffix[0] != '/' {
		return false
	}
	return !isManagementPathInternal(c.Request().Method, suffix)
}

func isManagementPathInternal(method, suffix string) bool {
	if suffix == "/runtime-runs" || strings.HasPrefix(suffix, "/runtime-runs/") || suffix == "/runtime-registration" || suffix == "/runtime-target" {
		return true
	}
	if method == http.MethodPost {
		parts := strings.Split(strings.Trim(suffix, "/"), "/")
		if len(parts) == 3 && parts[1] != "" {
			if parts[0] == "projects" && (parts[2] == "up" || parts[2] == "down" || parts[2] == "restart" || parts[2] == "redeploy") {
				return true
			}
			if parts[0] == "containers" && (parts[2] == "start" || parts[2] == "stop" || parts[2] == "restart") {
				return true
			}
		}
	}

	if suffix == "/jobs" || strings.HasPrefix(suffix, "/jobs/") {
		return true
	}
	if isCentralVolumeBackupPolicyPathInternal(method, suffix) {
		return true
	}
	if suffix == "/activities" || strings.HasPrefix(suffix, "/activities/") {
		return true
	}

	if strings.HasPrefix(suffix, "/notifications") {
		return true
	}

	// Webhooks are managed centrally: rows live in the manager DB (keyed by the
	// real environment ID) and the public trigger endpoint resolves tokens there.
	if suffix == "/webhooks" || strings.HasPrefix(suffix, "/webhooks/") {
		return true
	}

	if strings.HasPrefix(suffix, "/deployment/mtls/") {
		return true
	}

	_, isManagement := managementEndpointSet[suffix]
	return isManagement
}

func isCentralVolumeBackupPolicyPathInternal(method, suffix string) bool {
	if method != http.MethodGet && method != http.MethodPut {
		return false
	}
	parts := strings.Split(strings.Trim(suffix, "/"), "/")
	return len(parts) == 3 && parts[0] == "volumes" && parts[1] != "" && parts[2] == "backup-policy"
}

// extractEnvironmentID gets the environment ID from the request.
func (m *EnvironmentMiddleware) extractEnvironmentID(c *echo.Context) string {
	requestPath := c.Request().URL.Path

	if !strings.Contains(requestPath, environmentsPathMarker) {
		return ""
	}

	if envID := c.Param(m.paramName); envID != "" {
		return envID
	}

	if _, rest, ok := strings.Cut(requestPath, environmentsPathMarker); ok {
		if envID, _, _ := strings.Cut(rest, "/"); envID != "" {
			return envID
		}
	}

	return ""
}

// buildResourceSuffix extracts the resource path after stripping the environment ID prefix.
func (m *EnvironmentMiddleware) buildResourceSuffix(requestPath, envID string) string {
	suffix, _ := strings.CutPrefix(requestPath, apiEnvironmentsPrefix+envID)
	if suffix != "" && suffix[0] != '/' {
		suffix = "/" + suffix
	}
	return suffix
}

// buildTargetURL constructs the full proxy target URL for a remote environment.
func (m *EnvironmentMiddleware) buildTargetURL(c *echo.Context, envID, apiURL string) string {
	req := c.Request()
	suffix := m.buildResourceSuffix(req.URL.Path, envID)
	target := strings.TrimRight(apiURL, "/") + path.Join(apiEnvironmentsPrefix, m.localID) + suffix
	if qs := req.URL.RawQuery; qs != "" {
		target += "?" + qs
	}
	return target
}

// buildProxyPath constructs the path sent through the edge tunnel.
func (m *EnvironmentMiddleware) buildProxyPath(c *echo.Context, envID string) string {
	return path.Join(apiEnvironmentsPrefix, m.localID) + m.buildResourceSuffix(c.Request().URL.Path, envID)
}

func isEdgeEnvironmentURLInternal(apiURL string) bool {
	normalized := strings.ToLower(strings.TrimSpace(apiURL))
	return strings.HasPrefix(normalized, "edge://")
}

func (m *EnvironmentMiddleware) getActiveEdgeTunnelInternal(envID string) mo.Option[*edge.AgentTunnel] {
	if m.registry == nil {
		return mo.None[*edge.AgentTunnel]()
	}

	tunnel, ok := m.registry.Get(envID).Get()
	if !ok || tunnel == nil || tunnel.Conn == nil || tunnel.Conn.IsClosed() {
		return mo.None[*edge.AgentTunnel]()
	}
	return mo.Some(tunnel)
}

func (m *EnvironmentMiddleware) waitForActiveEdgeTunnelInternal(ctx context.Context, envID string, timeout time.Duration) mo.Option[*edge.AgentTunnel] {
	if timeout <= 0 {
		return m.getActiveEdgeTunnelInternal(envID)
	}

	if tunnel, ok := m.getActiveEdgeTunnelInternal(envID).Get(); ok {
		return mo.Some(tunnel)
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(edge.DefaultTunnelAcquirePollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return mo.None[*edge.AgentTunnel]()
		case <-ticker.C:
			if tunnel, ok := m.getActiveEdgeTunnelInternal(envID).Get(); ok {
				return mo.Some(tunnel)
			}
		}
	}
}

func (m *EnvironmentMiddleware) abortEdgeTunnelUnavailable(c *echo.Context) error {
	return c.JSON(http.StatusBadGateway, map[string]any{
		"success": false,
		"code":    "agent.unavailable",
		"data": map[string]any{
			"error": "Agent is not connected",
		},
	})
}

// proxyWebSocket handles WebSocket proxy requests.
func (m *EnvironmentMiddleware) proxyWebSocket(c *echo.Context, target string, accessToken *string, envID string) error {
	if isEdgeEnvironmentURLInternal(target) {
		slog.WarnContext(c.Request().Context(), "Refusing direct websocket proxy to edge environment without active tunnel", "environmentId", envID, "target", target)
		return m.abortEdgeTunnelUnavailable(c)
	}

	wsTarget := edge.HTTPToWebSocketURL(target)
	headers := edge.BuildWebSocketHeaders(c, accessToken)

	if err := ws.ProxyHTTP(c.Response(), c.Request(), wsTarget, headers, m.checkOrigin); err != nil {
		slog.ErrorContext(c.Request().Context(), "websocket proxy failed", "err", err)
	}
	return nil
}

// proxyHTTP handles standard HTTP proxy requests.
func (m *EnvironmentMiddleware) proxyHTTP(c *echo.Context, target string, accessToken *string) error {
	edge.CopyResponseHeaders(nil, c.Response().Header(), nil)
	if isEdgeEnvironmentURLInternal(target) {
		slog.WarnContext(c.Request().Context(), "Refusing direct HTTP proxy to edge environment without active tunnel", "target", target)
		return m.abortEdgeTunnelUnavailable(c)
	}

	req, err := m.createProxyRequest(c, target, accessToken)
	if err != nil {
		errMessage := "Failed to create proxy request: " + err.Error()
		if errors.Is(err, common.ErrEnvironmentInvalidProxyTarget) {
			errMessage = err.Error()
		}
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"success": false,
			"data":    map[string]any{"error": errMessage},
		})
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]any{
			"success": false,
			"code":    "agent.unavailable",
			"data":    map[string]any{"error": "Proxy request failed: " + err.Error()},
		})
	}
	defer func() { _ = resp.Body.Close() }()

	return m.writeProxyResponse(c, resp)
}

// createProxyRequest builds the HTTP request to forward to the remote environment.
func (m *EnvironmentMiddleware) createProxyRequest(c *echo.Context, target string, accessToken *string) (*http.Request, error) {
	srcReq := c.Request()
	validatedTarget, err := httpx.ValidateOutboundHTTPURL(target)
	if err != nil {
		return nil, common.Classify(common.ErrEnvironmentInvalidProxyTarget, fmt.Errorf("Invalid proxy target URL: %w", err)) //nolint:staticcheck // Preserve the existing error message.
	}

	// The body is streamed straight through rather than buffered: volume backup
	// and image import uploads run to gigabytes, and reading them into memory
	// cost roughly twice their size per in-flight request. This drops GetBody,
	// so the transport can no longer replay the body across a redirect or an
	// idle-connection retry; a proxied API call has no legitimate need for
	// either, and a failure surfaces as a 502 rather than silent corruption.
	//
	// A server request's ContentLength is 0 only when there is genuinely no
	// body (-1 means chunked/unknown), so it is the safe signal for whether to
	// forward one at all.
	var requestBody io.ReadCloser
	var contentLength int64
	switch {
	case srcReq.Body == nil:
	case srcReq.ContentLength != 0:
		requestBody, contentLength = srcReq.Body, srcReq.ContentLength
	default:
		_ = srcReq.Body.Close()
	}

	// The body is deliberately not logged: at debug level it would put compose
	// files and registry credentials into the ring buffer served by
	// /api/diagnostics/logs, and stringifying it allocated even when debug was off.
	slog.DebugContext(srcReq.Context(), "Creating proxy request", "method", srcReq.Method, "target", target, "contentLength", srcReq.ContentLength, "contentType", srcReq.Header.Get("Content-Type"))

	requestURL := *validatedTarget
	req := (&http.Request{
		Method:        srcReq.Method,
		URL:           &requestURL,
		Host:          requestURL.Host,
		Header:        make(http.Header),
		Body:          requestBody,
		ContentLength: contentLength,
	}).WithContext(srcReq.Context())

	skip := edge.GetSkipHeaders()
	edge.CopyRequestHeaders(srcReq.Header, req.Header, skip)
	remenv.StripHubCredentials(req.Header, m.proxyIdentityHeader)
	edge.SetAgentToken(req, accessToken)
	edge.SetForwardedHeaders(req, c.RealIP(), srcReq.Host)

	return req, nil
}

// writeProxyResponse copies the proxy response back to the client. An upstream
// 401 means the agent rejected the manager's credentials; passing it through
// makes browser clients treat it as a lost session, so it becomes a 502.
func (m *EnvironmentMiddleware) writeProxyResponse(c *echo.Context, resp *http.Response) error {
	if remenv.AgentAuthRejected(resp.StatusCode) {
		slog.WarnContext(c.Request().Context(), "Remote environment rejected the manager's credentials", "target", resp.Request.URL.Host)
		return c.JSON(http.StatusBadGateway, map[string]any{
			"success": false,
			"code":    "agent.invalid-token",
			"data":    map[string]any{"error": "Remote environment rejected the manager's credentials"},
		})
	}
	w := c.Response()
	hopByHop := edge.BuildHopByHopHeaders(resp.Header)
	edge.CopyResponseHeaders(resp.Header, w.Header(), hopByHop)

	w.WriteHeader(resp.StatusCode)
	if c.Request().Method != http.MethodHead {
		edge.CopyBodyWithFlush(w, resp.Body)
	}
	return nil
}

// EnvironmentProxySecurity supplies hub-owned identity and agent access boundaries.
type EnvironmentProxySecurity struct {
	Snapshots           *hoststate.Store
	InventoryPending    func(context.Context, string) (bool, error)
	HTTPClient          *http.Client
	ProxyIdentityHeader string
	AgentAccess         remenv.AgentAccessCheck
}
