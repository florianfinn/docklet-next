// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original M1/M2/M5 module access, gate and dispatch proofs.

package bootstrap

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/capability"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	authDTO "github.com/florianfinn/docklet-next/types/auth"
	"github.com/florianfinn/docklet-next/types/features"
	"github.com/stretchr/testify/require"
)

func TestM27M1HubModuleGateAccessOrder(t *testing.T) {
	x := newM27ModuleFixture(t, "direct", true)
	u, err := x.f.users.CreateUser(t.Context(), &user.User{Username: "module-viewer"})
	require.NoError(t, err)
	login, err := x.f.svc.CompleteLogin(t.Context(), u, authDTO.SessionMeta{}, "local")
	require.NoError(t, err)
	for _, path := range []string{"/api/modules/proxy/probe", "/api/environments/0/modules/proxy/data"} {
		unknown := m27UnknownResponse("DELETE", path)
		admin := x.request("DELETE", path, "")
		require.Equal(t, 404, admin.Code)
		require.Equal(t, unknown.Body.String(), admin.Body.String())
		r := httptest.NewRequest("DELETE", path, nil)
		r.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: login.BrowserToken})
		w := httptest.NewRecorder()
		x.hub.ServeHTTP(w, r)
		require.Equal(t, 403, w.Code)
		require.Contains(t, w.Body.String(), "access.admin-required")
	}
	require.Zero(t, x.hubCalls.Load())
	require.Zero(t, x.calls.Load())
}
func TestM27M2ModuleGateRealDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			x := newM27ModuleFixture(t, mode, true)
			for _, path := range []string{m27HostProbe, "/api/environments/example/modules/%70roxy/probe"} {
				unknown := m27UnknownResponse("DELETE", path)
				out := x.request("DELETE", path, "")
				require.Equal(t, 404, out.Code, path, out.Body.String())
				require.Equal(t, unknown.Body.String(), out.Body.String())
				require.Zero(t, x.calls.Load())
			}
			// Hub on, agent off: the real sender/receiver preserves the typed refusal.
			require.NoError(t, x.hubSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
			out := x.request("DELETE", m27HostProbe, "")
			require.Equal(t, 403, out.Code, out.Body.String())
			require.Contains(t, out.Body.String(), `"code":"feature-disabled"`)
			require.Contains(t, out.Body.String(), `"feature":"proxy"`)
			require.Zero(t, x.calls.Load())
			require.NoError(t, x.agentSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
			out = x.request("DELETE", m27HostProbe, "")
			require.Equal(t, 200, out.Code, out.Body.String())
			require.EqualValues(t, 1, x.calls.Load())
		})
	}
}
func TestM27M5ModuleCompatibilityRealDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, boundary := range []string{"v18", "fingerprint", "capability"} {
			if mode == "direct" && boundary == "capability" {
				continue
			}
			t.Run(mode+"/"+boundary, func(t *testing.T) {
				x := newM27ModuleFixture(t, mode, true)
				require.NoError(t, x.hubSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
				require.NoError(t, x.agentSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
				switch boundary {
				case "v18":
					x.version.Store(18)
				case "fingerprint":
					x.wrongFingerprint.Store(true)
				case "capability":
					// No request has used this tunnel yet; remove the advertised module bit.
					capabilities := []string{}
					for _, c := range x.tunnel.Capabilities {
						if c != "module.proxy" {
							capabilities = append(capabilities, c)
						}
					}
					x.tunnel.Capabilities = capabilities
				}
				out := x.request("DELETE", m27HostProbe, "")
				require.Equal(t, 409, out.Code, out.Body.String())
				require.Contains(t, out.Body.String(), "agent.update-required")
				require.Zero(t, x.calls.Load())
				require.Equal(t, "agent.update-required", x.hosts.ModuleReason(t.Context(), "example", features.Proxy))
				capResponse := x.request("GET", "/api/capabilities?kind=host&id=example", "")
				require.Equal(t, 200, capResponse.Code, capResponse.Body.String())
				var descriptors base.ApiResponse[[]capability.Descriptor]
				require.NoError(t, json.Unmarshal(capResponse.Body.Bytes(), &descriptors))
				found := false
				for _, d := range descriptors.Data {
					if d.OperationID == "module.proxy" {
						found = true
						require.False(t, d.Available)
						require.Equal(t, "agent.update-required", d.DisabledReasonCode)
					}
				}
				require.True(t, found)
				if boundary != "fingerprint" {
					_, status, err := x.hosts.ProxyRequest(t.Context(), "example", "GET", "/api/environments/0/containers", nil)
					require.NoError(t, err)
					require.Equal(t, 200, status, "v18 and missing-module hosts retain ordinary host operations")
				}
			})
		}
	}
}

func TestM27M5LegacyHostsKeepOrdinarySettingsSync(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			x := newM27ModuleFixture(t, mode, false)
			x.version.Store(18)
			require.NoError(t, x.agentSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
			syncTimeoutSettingsToAgentsInternal(t.Context(), x.hosts, []libarcane.SettingUpdate{{Key: features.ProxySettingKey, Value: "false"}, {Key: "activityEventLimit", Value: "50"}})
			require.True(t, x.agentSettings.IsFeatureEnabled(t.Context(), features.Proxy), "no unsupported module setting reaches a v18 host")
			require.Equal(t, 50, x.agentSettings.GetIntSetting(t.Context(), "activityEventLimit", 0))
		})
	}
}
