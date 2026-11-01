// Source: docklet-next/docs/design/resource-rights.md
// Revision: b1c605f029423b3fe2d3931227db84cb5fd8eecb
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original real Direct/Edge demotion coverage for sessionless administrators (S1 verify 2 finding S1V2-1).

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	keyDTO "github.com/florianfinn/docklet-next/types/apikey"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

// A demoted administrator authenticated by personal key or proxy identity loses its open stream.
func TestSessionlessAdministratorDemotionClosesRealStream(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			for _, credential := range []string{"personal-key", "proxy"} {
				t.Run(credential, func(t *testing.T) {
					x := newM15Transport(t, mode)
					f := x.fixture
					u, err := f.users.CreateUser(t.Context(), &user.User{Username: "target-admin"})
					require.NoError(t, err)
					require.NoError(t, f.users.SetAdministrator(t.Context(), f.adminID, u.ID, true))
					key, err := f.keys.CreatePersonalApiKey(t.Context(), u.ID, keyDTO.CreateUserApiKey{Name: "target"})
					require.NoError(t, err)
					headers := http.Header{"Origin": {f.cfg.AppUrl}}
					if credential == "proxy" {
						f.cfg.AuthMode = "proxy"
						f.cfg.AuthTrustedProxies = "127.0.0.1"
						f.cfg.AuthProxyAdminUsers = new("Operator")
						headers.Set("Remote-User", u.Username)
					} else {
						f.cfg.AuthMode = "local"
						headers.Set(middleware.HeaderApiKey, key.Key)
					}
					hub := httptest.NewServer(x.hub)
					defer hub.Close()
					ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
					defer cancel()
					conn, _, err := websocket.Dial(ctx, edge.HTTPToWebSocketURL(hub.URL)+"/api/environments/example/ws/containers/canonical/logs", &websocket.DialOptions{HTTPHeader: headers})
					require.NoError(t, err)
					defer conn.CloseNow()
					_, body, err := conn.Read(ctx)
					require.NoError(t, err)
					require.Equal(t, "ready", string(body))
					admin, err := f.users.GetUserByID(t.Context(), f.adminID)
					require.NoError(t, err)
					actorCtx := context.WithValue(t.Context(), userDTO.CurrentUserContextKey{}, admin)
					require.NoError(t, f.roles.SetUserAssignments(actorCtx, u.ID, []role.UserRoleAssignment{}))
					ps, err := f.roles.ResolvePermissions(t.Context(), u.ID)
					require.NoError(t, err)
					require.False(t, ps.IsGlobalAdmin())
					require.Empty(t, ps.Assignments, "the demoted account has no permitted stacks")
					request, err := http.NewRequestWithContext(ctx, http.MethodGet, hub.URL+"/api/environments/example/projects/foreign", nil)
					require.NoError(t, err)
					request.Header = headers.Clone()
					response, err := http.DefaultClient.Do(request)
					require.NoError(t, err)
					_ = response.Body.Close()
					require.Equal(t, http.StatusNotFound, response.StatusCode, "a new request is denied using current user rights")
					_ = conn.Write(ctx, websocket.MessageText, []byte("after role demotion"))
					_, body, err = conn.Read(ctx)
					require.NoError(t, ctx.Err(), "closure must precede the deadline")
					require.Error(t, err, "a demoted sessionless administrator must lose its stream; got %q", string(body))
				})
			}
		})
	}
}
