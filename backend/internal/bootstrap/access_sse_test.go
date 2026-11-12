// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package bootstrap

import (
	"bufio"
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/api/handlers"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	keyDTO "github.com/florianfinn/docklet-next/types/apikey"
	authDTO "github.com/florianfinn/docklet-next/types/auth"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestUserSSECookieProxyAndKeyLease(t *testing.T) {
	for _, credential := range []string{"cookie", "proxy", "personal-key"} {
		t.Run(credential, func(t *testing.T) {
			x := newM15Transport(t, "direct")
			f := x.fixture
			u, err := f.users.CreateUser(t.Context(), &user.User{Username: "viewer"})
			require.NoError(t, err)
			require.NoError(t, f.users.SetAppAssignments(t.Context(), u.ID, []userDTO.AppAssignment{{EnvironmentID: "example", ProjectID: "project-uuid", Level: "full"}}))
			tokens, err := f.svc.CompleteLogin(t.Context(), u, authDTO.SessionMeta{}, "local")
			require.NoError(t, err)
			key, err := f.keys.CreatePersonalApiKey(t.Context(), u.ID, keyDTO.CreateUserApiKey{Name: "viewer"})
			require.NoError(t, err)
			router := x.hub.Handler.(*echo.Echo)
			cfg := huma.DefaultConfig("streams", "test")
			cfg.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(t reflect.Type, hint string) string {
				for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
					t = t.Elem()
				}
				return t.PkgPath() + "." + huma.DefaultSchemaNamer(t, hint)
			})
			api := humaecho.NewWithGroup(router, router.Group("/api"), cfg)
			api.UseMiddleware(auth.NewHumaMiddleware(api, f.svc, f.keys, f.roles, x.hosts, f.cfg))
			api.UseMiddleware(f.svc.ViewerMiddleware(api))
			api.UseMiddleware(middleware.AccessMiddleware(api))
			handlers.RegisterStream(api, nil, nil, environment.NewHandler(x.hosts, nil, f.keys, nil, f.cfg, nil), nil, nil)
			f.cfg.AuthTrustedProxies = "127.0.0.1"
			f.cfg.AuthProxyAdminUsers = new("Operator")
			if credential == "proxy" {
				f.cfg.AuthMode = "proxy"
			}
			hub := httptest.NewServer(router)
			defer hub.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			r, err := http.NewRequestWithContext(ctx, "GET", hub.URL+"/api/stream?channels=environments", nil)
			require.NoError(t, err)
			switch credential {
			case "cookie":
				r.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: tokens.BrowserToken})
			case "proxy":
				r.Header.Set("Remote-User", u.Username)
			case "personal-key":
				r.Header.Set(middleware.HeaderApiKey, key.Key)
			}
			response, err := hub.Client().Do(r)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, 200, response.StatusCode)
			reader := bufio.NewReader(response.Body)
			for {
				line, err := reader.ReadString('\n')
				require.NoError(t, err)
				if strings.HasPrefix(line, "data:") {
					require.Contains(t, line, `"channel":"environments"`)
					require.NotContains(t, line, "apiUrl")
					break
				}
			}
			require.NoError(t, f.users.SetAppAssignments(t.Context(), u.ID, nil))
			_, err = io.ReadAll(reader)
			require.NoError(t, err)
			require.NoError(t, ctx.Err(), "revoked SSE should finish before deadline")
		})
	}
}
