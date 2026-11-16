// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Compatibility and registration gates before reconnect recovery.

package environment

import (
	"context"
	"testing"

	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

func TestReconnectedAgentRecoversOnlyCompatibleSession(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, status := range []string{"compatible", "incompatible", "disabled"} {
			t.Run(mode+"/"+status, func(t *testing.T) {
				f := newAgentFixture(t, mode, version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
				if status == "incompatible" {
					f.info.AgentContractVersion = 0
				}
				if status == "disabled" {
					require.NoError(t, f.service.db.Model(&Environment{}).Where("id = ?", f.id).UpdateColumn("enabled", false).Error)
				}
				calls := 0
				f.service.SetRuntimeRecovery(func(_ context.Context, id string) error { require.Equal(t, f.id, id); calls++; return nil })
				err := f.service.CheckReconnectedAgent(t.Context(), f.id)
				if status == "compatible" {
					require.NoError(t, err)
					require.Equal(t, 1, calls)
				} else {
					require.Error(t, err)
					require.Zero(t, calls)
				}
			})
		}
	}
}
