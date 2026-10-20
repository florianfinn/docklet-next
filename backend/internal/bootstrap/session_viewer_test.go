// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original early environment-proxy session context regression.

package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestSessionEnvironmentValidatorPreservesLocalAndUnboundIdentities(t *testing.T) {
	for _, credential := range []string{"cookie", "bearer", "api-key", "none", "proxy"} {
		t.Run(credential, func(t *testing.T) {
			f := newSignInRoutingFixture(t)
			_, id, err := f.svc.VerifyToken(t.Context(), f.tokens.AccessToken)
			require.NoError(t, err)
			req := httptest.NewRequest("GET", "/api/environments/example/ws/system/stats", nil)
			req.RemoteAddr = "192.0.2.10:5000"
			switch credential {
			case "cookie":
				req.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: f.tokens.BrowserToken})
			case "bearer":
				req.Header.Set("Authorization", "Bearer "+f.tokens.AccessToken)
			case "api-key":
				req.Header.Set("X-Api-Key", f.personalKey)
			case "none", "proxy":
				f.cfg.AuthMode = credential
				req.Header.Set("Remote-User", "example-operator")
			}
			router := echo.New()
			c := router.NewContext(req, httptest.NewRecorder())
			validator := createAuthValidatorInternal(api.HandlerDeps{Auth: auth.New(f.svc, nil, nil), ApiKey: apikey.New(f.keys), Role: role.New(f.roles)})
			_, _, ok := validator(req.Context(), c)
			require.True(t, ok)
			closed := false
			l, err := sessionlife.Register(c.Request().Context(), func() { closed = true })
			require.NoError(t, err)
			defer l.Release()
			require.NoError(t, f.svc.RevokeSession(t.Context(), id))
			require.Equal(t, credential == "cookie" || credential == "bearer", closed)
		})
	}
}
