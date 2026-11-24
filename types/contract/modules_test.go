// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original v19 module registry and compatibility contract checks.

package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/modules"
	"github.com/stretchr/testify/require"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestModuleV20Foundation(t *testing.T) {
	require.Equal(t, 23, AgentHubVersion)
	require.Equal(t, 18, MinimumAgentVersion)
	require.True(t, AgentCompatible(18, AgentHubV18Fingerprint))
	require.False(t, ModuleCompatible(18, AgentHubFingerprint))
	require.False(t, ModuleCompatible(19, "different"))
	require.False(t, ModuleCompatible(19, AgentHubFingerprint))
	require.True(t, ModuleCompatible(23, AgentHubFingerprint))
	require.True(t, ModuleCompatible(21, AgentHubV21Fingerprint), "v22 changes no module request")
	require.True(t, ModuleCompatible(22, AgentHubV22Fingerprint), "v23 changes no agent module request")
	require.True(t, ModuleCompatible(20, AgentHubV20Fingerprint), "v21 changes no module request")
	require.False(t, ModuleCompatible(20, AgentHubFingerprint))
	legacy, err := os.ReadFile("testdata/agent-hub-v18.json")
	require.NoError(t, err)
	sum := sha256.Sum256(legacy)
	require.Equal(t, AgentHubV18Fingerprint, hex.EncodeToString(sum[:]))
	raw, err := os.ReadFile("testdata/agent-hub-v23.json")
	require.NoError(t, err)
	var fixture struct {
		Modules struct {
			Definitions []struct {
				ID                                                         features.ID
				SettingKey, HubPrefix, HostPrefix, Capability, AgentModule string
				DefaultEnabled                                             bool
			}
			RefusalCodes    []string
			MinimumContract int
		}
	}
	require.NoError(t, json.Unmarshal(raw, &fixture))
	require.Equal(t, 20, fixture.Modules.MinimumContract)
	require.Len(t, fixture.Modules.Definitions, len(features.Modules()))
	for i, d := range features.Modules() {
		got := fixture.Modules.Definitions[i]
		require.Equal(t, d.ID, got.ID)
		require.Equal(t, d.SettingKey, got.SettingKey)
		require.Equal(t, d.HubPrefix, got.HubPrefix)
		require.Equal(t, d.HostPrefix, got.HostPrefix)
		require.Equal(t, d.Capability, got.Capability)
		require.Equal(t, d.AgentModule, got.AgentModule)
		require.Equal(t, d.DefaultEnabled, got.DefaultEnabled)
	}
	field, _ := reflect.TypeFor[dto.Problem]().FieldByName("Code")
	require.Equal(t, strings.Split(field.Tag.Get("enum"), ","), fixture.Modules.RefusalCodes)
}
