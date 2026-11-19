// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original request policy shared by API, streams and environment proxy; M15 simple-user rights and identity contracts; M29 stored policy snapshot and setup-pending gate.

package auth

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
)

func (s *AuthService) SignInMode() authtypes.SignInMode {
	return s.CurrentPolicy().Mode
}

// ModeRequestUser authenticates the direct peer; forwarded addresses never establish identity.
// Local mode leaves credential verification to the existing session/API-key paths.
func (s *AuthService) ModeRequestUser(ctx context.Context, req *http.Request) (*user.User, error) {
	return s.modeRequestUserInternal(ctx, req, s.CurrentPolicy())
}

func (s *AuthService) modeRequestUserInternal(ctx context.Context, req *http.Request, p *Policy) (*user.User, error) {
	if p.Pending {
		return nil, ErrSetupPending
	}
	var identity string
	switch p.Mode {
	case authtypes.SignInModeNone:
	case authtypes.SignInModeProxy:
		var err error
		if identity, err = proxyIdentityInternal(req, p); err != nil {
			return nil, err
		}
	default:
		return nil, common.ErrInvalidToken
	}
	// An empty or blank list counts as unset, so uncommenting the line without a value keeps today's mapping.
	if identity != "" && s.config.AuthProxyAdminUsers != nil && strings.TrimSpace(*s.config.AuthProxyAdminUsers) != "" {
		listed := false
		for _, name := range strings.Split(*s.config.AuthProxyAdminUsers, ",") {
			listed = listed || strings.TrimSpace(name) == identity
		}
		if !listed {
			u, err := s.userService.GetUserByUsername(ctx, identity)
			if err != nil {
				return nil, errors.New("access.not-registered")
			}
			if u.DisabledAt != nil {
				return nil, errors.New("access.disabled")
			}
			return u, nil
		}
	}
	admin, err := s.userService.GlobalAdministrator(ctx)
	if err != nil {
		return nil, err
	}
	if identity != "" {
		// Keep the persisted administrator ID; only activity attribution uses the proxy name.
		admin.Username = identity
	}
	return admin, nil
}

// proxyIdentityInternal returns the single identity header value of a trusted direct peer.
func proxyIdentityInternal(req *http.Request, p *Policy) (string, error) {
	if !trustedIdentityPeerInternal(req.RemoteAddr, p.Networks) {
		return "", common.ErrInvalidToken
	}
	values := req.Header.Values(p.Header)
	if len(values) != 1 {
		return "", common.ErrInvalidToken
	}
	if !utf8.ValidString(values[0]) || strings.IndexFunc(values[0], func(ch rune) bool { return !unicode.IsPrint(ch) }) != -1 {
		return "", common.ErrInvalidToken
	}
	identity := strings.TrimSpace(values[0])
	if identity == "" || len(identity) > 255 {
		return "", common.ErrInvalidToken
	}
	return identity, nil
}

func trustedIdentityPeerInternal(remoteAddr string, networks []netip.Prefix) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || peer.Zone() != "" {
		return false
	}
	for _, network := range networks {
		if network.Contains(peer.Unmap()) {
			return true
		}
	}
	return false
}

func isLocalCredentialPathInternal(path string) bool {
	return strings.HasSuffix(path, "/auth/login") || strings.HasSuffix(path, "/auth/refresh") || strings.HasSuffix(path, "/auth/password")
}

func handleModeAuthInternal(api huma.API, ctx huma.Context, service *AuthService, resolver EnvironmentAccessTokenResolver, next func(huma.Context)) bool {
	p := service.CurrentPolicy()
	if p.LocalActive() {
		return false
	}
	if isLocalCredentialPathInternal(ctx.URL().Path) {
		if p.Pending {
			_ = access.WriteProblem(ctx, http.StatusUnauthorized, authtypes.CodeSetupPending)
		} else {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Local authentication is disabled")
		}
		return true
	}
	if !parseSecurityRequirementsInternal(api, ctx).isRequired {
		if p.Pending {
			// Public operations run without a user; no opportunistic credential while setup is pending.
			next(ctx)
			return true
		}
		return false
	}
	for _, header := range []string{middleware.HeaderAgentToken, middleware.HeaderApiKey} {
		if transportUser, env, ok := tryEnvironmentAccessTokenAuthInternal(ctx, resolver, ctx.Header(header)); ok {
			applyProxiedIconCatalogInternal(ctx, transportUser)
			next(huma.WithContext(ctx, setUserInContextInternal(ctx.Context(), transportUser, authz.EnvironmentPermissionSet(env.ID))))
			return true
		}
	}
	requestUser, err := service.modeRequestUserInternal(ctx.Context(), humaecho.Unwrap(ctx).Request(), p)
	if err != nil {
		status, code := modeAuthError(err)
		if strings.HasPrefix(code, "access.") || strings.HasPrefix(code, "setup.") {
			_ = access.WriteProblem(ctx, status, code)
		} else {
			_ = huma.WriteErr(api, ctx, status, code)
		}
		return true
	}
	next(huma.WithContext(ctx, setUserInContextInternal(ctx.Context(), requestUser, service.modePermissions(ctx.Context(), requestUser))))
	return true
}

func (s *AuthService) IsLocalAuthEnabled(_ context.Context) (bool, error) {
	if s == nil {
		return false, errors.New("authentication service unavailable")
	}
	return s.CurrentPolicy().LocalActive(), nil
}

func (s *AuthService) InitializeSignIn(ctx context.Context) error {
	if err := s.config.ValidateSignIn(); err != nil {
		return err
	}
	if s.config.AgentMode || s.config.EdgeAgent {
		return nil
	}
	// User handlers follow the runtime policy once it is published.
	s.userService.SetSignInModeSource(s.SignInMode)
	p, err := s.loadPolicyInternal(ctx)
	if err != nil {
		// Recovery replaces an unusable stored policy with local sign-in (recoverAdministrator stores it).
		_, forced := s.config.ForcedSignInMode()
		if s.config.AuthResetAdminPassword == "" || forced {
			return err
		}
		_, claimed, stateErr := s.sessionService.LoadSignInState(ctx)
		if stateErr != nil {
			return stateErr
		}
		p = &Policy{Mode: authtypes.SignInModeLocal, Header: defaultProxyHeader, Pending: !claimed}
	}
	if err := s.sessionService.ActivateSignInMode(ctx, p.Mode); err != nil {
		return err
	}
	s.policy.Store(p)
	s.InvalidateAllTokenCache()
	if p.Pending {
		s.startSetupInternal(ctx)
		return nil
	}
	return s.recoverAdministrator(ctx)
}

// Source: getarcaneapp/arcane/backend/internal/auth/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Relocated without behavioural changes.
func (s *AuthService) cachedVerificationInternal(tokenHash string) (*user.User, string, bool) {
	cached, ok, _ := s.tokenCache.Get(tokenHash)
	if !ok {
		return nil, "", false
	}
	now := time.Now()
	if now.After(cached.TokenExpiresAt) || now.After(cached.SessionExpiresAt) {
		s.tokenCache.Delete(tokenHash)
		return nil, "", false
	}
	return new(cached.User), cached.SessionID, true
}
