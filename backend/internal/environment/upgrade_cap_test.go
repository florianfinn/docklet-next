// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Internal forwarding cap regressions across both real transport adapters.

package environment

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/types/contract"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExecuteRemoteRequestUpgradeCapBeforeTransport(t *testing.T) {
	saved := config.Version
	defer func() { config.Version = saved }()
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			f := newAgentFixture(t, mode, version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
			for _, test := range []struct{ hub, target, code string }{{"2.0.0", "2.1.0", "update.requires-hub-update"}, {"2.0.0", "", "update.invalid-target"}, {"custom", "1.0.0", "update.system-version-unknown"}} {
				config.Version = test.hub
				body, err := json.Marshal(map[string]string{"targetVersion": test.target})
				require.NoError(t, err)
				_, err = f.service.ExecuteRemoteRequest(t.Context(), f.id, "POST", "/api/environments/0/system/upgrade", body)
				require.Error(t, err)
				require.Equal(t, test.code, imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
				require.Zero(t, f.count("POST /api/environments/0/system/upgrade"))
			}
			config.Version = "2.0.0"
			_, err := f.service.ExecuteRemoteRequest(t.Context(), f.id, "POST", "/api/environments/0/system/upgrade", []byte(`{"targetVersion":"2.0.0"}`))
			require.NoError(t, err)
			require.Equal(t, 1, f.count("POST /api/environments/0/system/upgrade"), "positive control observes the actual Direct/Edge effect")
		})
	}
}
