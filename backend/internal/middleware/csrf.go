// Source: getarcaneapp/arcane/backend/internal/middleware/csrf.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Explicit Origin and double-submit CSRF checks for every hub sign-in mode; independently authenticated transports retain their ingress.

package middleware

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
)

const CSRFHeader = "X-CSRF-Token"

type csrfTokenContextKey struct{}

func CSRFTokenFromContext(ctx context.Context) string {
	value, _ := ctx.Value(csrfTokenContextKey{}).(string)
	return value
}

// CSRFMiddleware requires an allowed Origin and a cookie/header token pair on hub writes.
// Credential exceptions require validation rather than header presence.
type CSRFMiddleware struct {
	cfg                    *config.Config
	transportAuthenticated func(*http.Request) bool
	localCredential        func(*http.Request) (bool, error)
}

func NewCSRFMiddleware(cfg *config.Config) *CSRFMiddleware {
	return &CSRFMiddleware{cfg: cfg}
}

func (m *CSRFMiddleware) WithTransportValidator(validate func(*http.Request) bool) *CSRFMiddleware {
	m.transportAuthenticated = validate
	return m
}

func (m *CSRFMiddleware) WithLocalCredentialValidator(validate func(*http.Request) (bool, error)) *CSRFMiddleware {
	m.localCredential = validate
	return m
}

func (m *CSRFMiddleware) Add() echo.MiddlewareFunc {
	origins := deriveAllowedOriginsInternal(m.cfg, nil)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			req := c.Request()
			if m.cfg != nil && (m.cfg.AgentMode || m.cfg.EdgeAgent) {
				return next(c)
			}
			if !strings.HasPrefix(req.URL.Path, "/api/") {
				return next(c)
			}
			switch req.Method {
			case http.MethodGet, http.MethodHead:
				token := requestCSRFToken(req)
				if token == "" {
					random := make([]byte, 32)
					if _, err := rand.Read(random); err != nil {
						return err
					}
					token = base64.RawURLEncoding.EncodeToString(random)
					http.SetCookie(c.Response(), &http.Cookie{Name: csrfCookieName(req), Value: token, Path: "/", Secure: cookie.SecureCookieFromContext(req.Context()), HttpOnly: true, SameSite: http.SameSiteStrictMode})
				}
				c.Response().Header().Set(CSRFHeader, token)
				c.Response().Header().Set("Cache-Control", "no-store")
				c.SetRequest(req.WithContext(context.WithValue(req.Context(), csrfTokenContextKey{}, token)))
				return next(c)
			case http.MethodOptions:
				return next(c)
			}
			// These routes authenticate only their own transport/path tokens downstream.
			cookieLess := len(req.Header.Values("Cookie")) == 0
			if strings.HasPrefix(req.URL.Path, "/api/webhooks/trigger/") || (cookieLess && m.transportAuthenticated != nil && m.transportAuthenticated(req)) {
				return next(c)
			}
			if m.cfg.SignInMode() == "local" {
				// These POSTs authenticate explicit body credentials and carry no ambient authority.
				if req.Method == http.MethodPost && req.Header.Get("Origin") == "" && cookieLess &&
					(req.URL.Path == "/api/auth/login" || req.URL.Path == "/api/auth/refresh") {
					return next(c)
				}
				// An operation may ignore a header credential and authenticate its cookie instead.
				if cookieLess && m.localCredential != nil {
					valid, err := m.localCredential(req)
					if err != nil {
						return c.JSON(http.StatusUnauthorized, common.APIError{Code: common.APIErrorCodeUnauthorized, Message: "Invalid or expired credential"})
					}
					if valid {
						return next(c)
					}
				}
			}
			originOK := false
			for _, allowed := range origins {
				if req.Header.Get("Origin") == allowed {
					originOK = true
					break
				}
			}
			if !originOK {
				return csrfForbidden(c, "Origin is required and must match the hub")
			}
			expected := requestCSRFToken(req)
			provided := req.Header.Get(CSRFHeader)
			if expected == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
				return csrfForbidden(c, "A valid CSRF token is required")
			}
			return next(c)
		}
	}
}

func csrfCookieName(req *http.Request) string {
	if cookie.SecureCookieFromContext(req.Context()) {
		return "__Host-docklet_csrf"
	}
	return "docklet_csrf"
}

func requestCSRFToken(req *http.Request) string {
	value, err := req.Cookie(csrfCookieName(req))
	if err != nil || len(value.Value) != 43 {
		return ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value.Value)
	if err != nil || len(decoded) != 32 {
		return ""
	}
	return value.Value
}

func csrfForbidden(c *echo.Context, message string) error {
	return c.JSON(http.StatusForbidden, common.APIError{Code: common.APIErrorCodeForbidden, Message: message})
}
