// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session-bound viewer and browser identity lifecycle implementation.

package auth

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestSessionAuthContextBridgeAndDurableIdentity(t *testing.T) {
	for _, surface := range []string{"huma", "echo"} {
		for _, credential := range []string{"cookie", "bearer"} {
			t.Run(surface+"/"+credential, func(t *testing.T) {
				svc, tokens, db := modeTestFixture(t)
				actor, id, err := svc.VerifyToken(t.Context(), tokens.AccessToken)
				require.NoError(t, err)
				router := echo.New()
				closed := false
				bind := func(ctx context.Context) {
					l, err := sessionlife.Register(ctx, func() { closed = true })
					require.NoError(t, err)
					t.Cleanup(l.Release)
				}
				if surface == "huma" {
					api := humaecho.New(router, huma.DefaultConfig("test", "1.0.0"))
					api.UseMiddleware(NewHumaMiddleware(api, svc, nil, svc.userServiceRoleResolver(), nil, svc.config))
					api.UseMiddleware(svc.ViewerMiddleware(api))
					huma.Register(api, huma.Operation{OperationID: "session-viewer", Method: "GET", Path: "/auth/me", Security: []map[string][]string{{"BearerAuth": {}}}}, func(ctx context.Context, _ *struct{}) (*secureOutput, error) { bind(ctx); return &secureOutput{}, nil })
				} else {
					router.Use(NewAuthMiddleware(svc, svc.config).WithPermissionResolver(svc.userServiceRoleResolver()).Add())
					router.GET("/auth/me", func(c *echo.Context) error { bind(c.Request().Context()); return c.NoContent(200) })
				}
				request := func() *httptest.ResponseRecorder {
					req := httptest.NewRequest("GET", "/auth/me", nil)
					if credential == "cookie" {
						req.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: tokens.BrowserToken})
					} else {
						req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
					}
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					return rec
				}
				require.Equal(t, 200, request().Code)
				require.NoError(t, session.NewSessionService(db).RevokeSession(t.Context(), id))
				require.True(t, closed, "authenticated context must reach shared registry")
				if surface == "huma" {
					// Verification cache may still contain the token; auth/me must read durable state.
					require.Equal(t, 401, request().Code)
				}
				require.NotEmpty(t, actor.ID)
			})
		}
	}
}

func TestSessionAuthPasswordRecoveryModeAndCleanupBypasses(t *testing.T) {
	for _, path := range []string{"password", "recovery", "mode", "cleanup"} {
		t.Run(path, func(t *testing.T) {
			svc, tokens, db := modeTestFixture(t)
			actor, id, err := svc.VerifyToken(t.Context(), tokens.AccessToken)
			require.NoError(t, err)
			ctx := svc.ViewerContext(t.Context(), id, actor.ID)
			closed := false
			l, err := sessionlife.Register(ctx, func() { closed = true })
			require.NoError(t, err)
			defer l.Release()
			switch path {
			case "password":
				current, err := svc.CompleteLogin(t.Context(), actor, authtypes.SessionMeta{}, session.UserSessionSourceLocal)
				require.NoError(t, err)
				_, currentID, err := svc.VerifyToken(t.Context(), current.AccessToken)
				require.NoError(t, err)
				require.NoError(t, svc.ChangePassword(t.Context(), actor.ID, "example-password", "Example-new-password1!", currentID))
				require.NoError(t, sessionlife.Check(svc.ViewerContext(t.Context(), currentID, actor.ID)), "current password-change session survives")
			case "recovery":
				cfg := *svc.config
				replacement := "Example-recovery-1!" + rand.Text()
				cfg.AuthResetAdminPassword = replacement
				restarted := NewAuthService(svc.userService, svc.settingsService, nil, session.NewSessionService(db), &cfg).WithSigningKey(svc.signingKey)
				require.NoError(t, restarted.recoverAdministrator(t.Context()))
			case "mode":
				require.NoError(t, svc.sessionService.ActivateSignInMode(t.Context(), "none"))
			case "cleanup":
				require.NoError(t, db.Model(&session.UserSession{}).Where("id = ?", id).Update("expires_at", time.Now().Add(-time.Hour)).Error)
				_, err := svc.sessionService.DeleteExpiredSessions(t.Context(), time.Hour)
				require.NoError(t, err)
			}
			require.False(t, closed, "bypassing transaction is detected by cadence, not before commit")
			require.Error(t, sessionlife.Check(ctx))
			l.Close()
			require.True(t, closed)
		})
	}
}

func TestSessionNilAuthViewerContext(t *testing.T) {
	var service *AuthService
	require.NoError(t, sessionlife.Check(service.ViewerContext(t.Context(), "", "")))
}
