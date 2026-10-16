// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original request policy shared by API, streams and environment proxy.

package auth

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
)

func (s *AuthService) SignInMode() authtypes.SignInMode {
	if s == nil {
		return authtypes.SignInModeLocal
	}
	return s.config.SignInMode()
}

// ModeRequestUser authenticates the direct peer; forwarded addresses never establish identity.
// Local mode leaves credential verification to the existing session/API-key paths.
func (s *AuthService) ModeRequestUser(ctx context.Context, req *http.Request) (*user.User, error) {
	var identity string
	switch s.SignInMode() {
	case authtypes.SignInModeNone:
	case authtypes.SignInModeProxy:
		if !s.trustedIdentityPeer(req.RemoteAddr) {
			return nil, common.ErrInvalidToken
		}
		values := req.Header.Values(s.config.ProxyIdentityHeader())
		if len(values) != 1 {
			return nil, common.ErrInvalidToken
		}
		identity = strings.TrimSpace(values[0])
		if identity == "" || len(identity) > 255 || strings.ContainsAny(identity, "\r\n\x00") {
			return nil, common.ErrInvalidToken
		}
	default:
		return nil, common.ErrInvalidToken
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

func (s *AuthService) trustedIdentityPeer(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || peer.Zone() != "" {
		return false
	}
	networks, err := s.config.AuthProxyNetworks()
	if err != nil {
		return false
	}
	for _, network := range networks {
		if network.Contains(peer.Unmap()) {
			return true
		}
	}
	return false
}

func handleModeAuthInternal(api huma.API, ctx huma.Context, service *AuthService, resolver EnvironmentAccessTokenResolver, next func(huma.Context)) bool {
	if service.SignInMode() == authtypes.SignInModeLocal {
		return false
	}
	if strings.HasSuffix(ctx.URL().Path, "/auth/login") || strings.HasSuffix(ctx.URL().Path, "/auth/refresh") || strings.HasSuffix(ctx.URL().Path, "/auth/password") {
		_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Local authentication is disabled")
		return true
	}
	for _, header := range []string{middleware.HeaderAgentToken, middleware.HeaderApiKey} {
		if transportUser, env, ok := tryEnvironmentAccessTokenAuthInternal(ctx, resolver, ctx.Header(header)); ok {
			applyProxiedIconCatalogInternal(ctx, transportUser)
			next(huma.WithContext(ctx, setUserInContextInternal(ctx.Context(), transportUser, authz.EnvironmentPermissionSet(env.ID))))
			return true
		}
	}
	requestUser, err := service.ModeRequestUser(ctx.Context(), humaecho.Unwrap(ctx).Request())
	if err != nil {
		_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Sign-in required")
		return true
	}
	next(huma.WithContext(ctx, setUserInContextInternal(ctx.Context(), requestUser, authz.SudoPermissionSet())))
	return true
}

func (s *AuthService) IsLocalAuthEnabled(_ context.Context) (bool, error) {
	if s == nil {
		return false, errors.New("authentication service unavailable")
	}
	return s.SignInMode() == authtypes.SignInModeLocal, nil
}

func (s *AuthService) InitializeSignIn(ctx context.Context) error {
	if err := s.config.ValidateSignIn(); err != nil {
		return err
	}
	if err := s.sessionService.ActivateSignInMode(ctx, s.SignInMode()); err != nil {
		return err
	}
	s.InvalidateAllTokenCache()
	return s.recoverAdministrator(ctx)
}

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
