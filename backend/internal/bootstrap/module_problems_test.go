// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original typed refusal fixtures produced by real Direct and Edge forwarding.

package bootstrap

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/modules"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestM27M7ProblemFixturesRealDirectEdge(t *testing.T) {
	type entry struct {
		Transport string          `json:"transport"`
		Code      string          `json:"code"`
		Status    int             `json:"status"`
		Body      json.RawMessage `json:"body"`
	}
	var entries []entry
	for _, mode := range []string{"direct", "edge"} {
		x := newM27ModuleFixture(t, mode, true)
		require.NoError(t, x.hubSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
		require.NoError(t, x.agentSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
		for _, code := range []string{"module-busy", "module-on", "operation-conflict", "feature-disabled", "module-not-found"} {
			switch code {
			case "feature-disabled":
				x.agentProblem.Store(nil)
				require.NoError(t, x.agentSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "false"))
			case "module-not-found":
				require.NoError(t, x.hubSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "false"))
			default:
				x.agentProblem.Store(&dto.Problem{Status: 409, Code: code, Feature: features.Proxy, Operation: "apply-1"})
			}
			out := x.request("DELETE", m27HostProbe, "")
			status := 409
			if code == "feature-disabled" {
				status = 403
			}
			if code == "module-not-found" {
				status = 404
			}
			require.Equal(t, status, out.Code, out.Body.String())
			if code != "module-not-found" {
				require.Contains(t, out.Body.String(), code)
			}
			entries = append(entries, entry{Transport: mode, Code: code, Status: out.Code, Body: json.RawMessage(out.Body.Bytes())})
		}
	}
	raw, err := json.MarshalIndent(entries, "", "  ")
	require.NoError(t, err)
	raw = append(raw, '\n')
	path := filepath.Join("..", "..", "..", "types", "modules", "testdata", "problems.json")
	if os.Getenv("UPDATE_MODULE_PROBLEMS") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, raw, 0644))
	}
	expected, err := os.ReadFile(path)
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(raw))
}
