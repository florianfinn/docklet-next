// Source: docklet-next/docs/design/resource-rights.md
// Revision: 7d9d97d9fcdc3d6c1673b27f4022512234c0edcf
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original real Direct/Edge host-removal revocation coverage (closing review finding CL-B-1).

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	keyDTO "github.com/florianfinn/docklet-next/types/apikey"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

// Removing a host closes a user's stream on a surviving host, but an administrator who kept
// assignment rows from before a promotion keeps it.
func TestHostRemovalRevokesUsersAndKeepsAdministratorStreams(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			for _, administrator := range []bool{true, false} {
				name := "user"
				if administrator {
					name = "administrator"
				}
				t.Run(name, func(t *testing.T) {
					x := newM15Transport(t, mode)
					f := x.fixture
					api.WireAssignmentCleanup(x.hosts, f.users)
					now := time.Now()
					require.NoError(t, f.db.Exec("INSERT INTO environments (id,name,api_url,status,enabled,created_at,updated_at) VALUES (?,?,?,?,?,?,?)", "removed-host", "Removed host", "https://removed.example.com", "offline", false, now, now).Error)
					u, err := f.users.CreateUser(t.Context(), &user.User{Username: "assigned-account"})
					require.NoError(t, err)
					require.NoError(t, f.users.SetAppAssignments(t.Context(), u.ID, []userDTO.AppAssignment{
						{EnvironmentID: "example", ProjectID: "project-uuid", Level: "restricted"},
						{EnvironmentID: "removed-host", ProjectID: "old-project", Level: "restricted"},
					}))
					if administrator {
						require.NoError(t, f.users.SetAdministrator(t.Context(), f.adminID, u.ID, true))
					}
					key, err := f.keys.CreatePersonalApiKey(t.Context(), u.ID, keyDTO.CreateUserApiKey{Name: "assigned"})
					require.NoError(t, err)
					headers := http.Header{"Origin": {f.cfg.AppUrl}, middleware.HeaderApiKey: {key.Key}}
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

					require.NoError(t, x.hosts.DeleteEnvironment(t.Context(), "removed-host", nil, nil))
					remaining, err := f.users.AppAssignments(t.Context(), u.ID)
					require.NoError(t, err)
					require.Len(t, remaining, 1, "only the surviving host's assignment remains")

					_ = conn.Write(ctx, websocket.MessageText, []byte("after host removal"))
					_, body, err = conn.Read(ctx)
					require.NoError(t, ctx.Err(), "the outcome must precede the deadline")
					if administrator {
						require.NoError(t, err, "an unchanged administrator keeps its stream on a surviving host")
						require.Equal(t, "after host removal", string(body))
						return
					}
					require.Error(t, err, "a user whose assignments changed loses its stream; got %q", string(body))
				})
			}
		})
	}
}
