// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original anonymous and simple-user forwarding order checks.

package bootstrap

import (
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	authDTO "github.com/florianfinn/docklet-next/types/auth"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestM27ModuleAuthenticationAndSimpleUserForwarding(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			x := newM27ModuleFixture(t, mode, true)
			for _, path := range []string{"/api/modules/proxy/probe", m27HostProbe} {
				w := httptest.NewRecorder()
				x.hub.ServeHTTP(w, httptest.NewRequest("DELETE", path, nil))
				require.Equal(t, 401, w.Code)
			}
			u, err := x.f.users.CreateUser(t.Context(), &user.User{Username: "module-viewer"})
			require.NoError(t, err)
			login, err := x.f.svc.CompleteLogin(t.Context(), u, authDTO.SessionMeta{}, "local")
			require.NoError(t, err)
			r := httptest.NewRequest("DELETE", m27HostProbe, nil)
			r.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: login.BrowserToken})
			w := httptest.NewRecorder()
			x.hub.ServeHTTP(w, r)
			require.Equal(t, 403, w.Code)
			require.Contains(t, w.Body.String(), "access.admin-required")
			require.Zero(t, x.calls.Load())
		})
	}
}
