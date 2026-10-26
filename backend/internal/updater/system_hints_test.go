// Source: docklet-next/docs/design/images-updates.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Pure release-evidence projection refuses above-hub, ambiguous and invented candidates.

package updater

import (
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	versionDTO "github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSystemImageHintsAreManualAndCapped(t *testing.T) {
	for _, test := range []struct {
		name, hub, agent, latest, best, reference, candidate, state string
		requires                                                    bool
	}{
		{"capped", "2.0.0", "1.9.0", "2.1.0", "2.0.0", "docklet-next-agent:1.9.0", "2.0.0", "available", true},
		{"no capped artifact", "2.0.0", "1.9.0", "2.1.0", "", "docklet-next-agent:1.9.0", "", "unknown", true},
		{"agent newer", "2.0.0", "2.1.0", "2.2.0", "2.0.0", "docklet-next-agent:2.1.0", "", "unknown", true},
		{"numeric ordering", "2.10.0", "2.9.0", "2.10.0", "2.10.0", "docklet-next-agent:2.9.0", "2.10.0", "available", false},
		{"prerelease", "2.0.0", "2.0.0-rc.1", "2.1.0", "2.0.0", "docklet-next-agent:2.0.0-rc.1", "", "unknown", false},
		{"custom reference", "2.0.0", "1.9.0", "2.1.0", "2.0.0", "registry.example.com/custom:1.9.0", "", "unknown", false},
		{"untrusted above cap", "2.0.0", "1.9.0", "2.1.0", "2.1.0", "docklet-next-agent:1.9.0", "", "current", true},
		{"no downgrade", "2.0.0", "1.9.1", "2.1.0", "1.9.0", "docklet-next-agent:1.9.1", "", "current", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			out := systemImageHints(test.hub, "docklet-next-agent:"+test.hub, versionDTO.Info{CurrentVersion: test.agent, CurrentImageRef: test.reference}, test.latest, test.best)
			require.Len(t, out.Images, 2)
			agent := out.Images[0]
			require.True(t, agent.ManualOnly)
			require.Equal(t, test.candidate, agent.CandidateVersion)
			require.Equal(t, test.requires, agent.RequiresHubUpdate)
			require.Equal(t, test.state, agent.State)
		})
	}
}
func TestSystemImageHintsCustomConfiguredReferenceRemainsUnknown(t *testing.T) {
	reference := "registry.example.com/custom-agent:1.9.0"
	out := systemImageHints("2.0.0", reference, versionDTO.Info{CurrentVersion: "1.9.0", CurrentImageRef: reference}, "2.1.0", "2.0.0")
	require.Equal(t, "unknown", out.Images[0].State)
	require.Empty(t, out.Images[0].CandidateVersion)
}

func TestSystemToolsHintPreservesPinsAndReleaseCompatibility(t *testing.T) {
	info := versionDTO.Info{CurrentVersion: "2.0.0", CurrentImageRef: "docklet-next-agent:2.0.0", ToolsImage: volumehelper.DefaultToolsImage}
	out := systemImageHints("2.0.0", "docklet-next-agent:2.0.0", info, "2.0.0", "2.0.0")
	require.Equal(t, "current", out.Images[1].State)
	require.Equal(t, volumehelper.DefaultToolsImage, out.Images[1].CompatibleReference)
	info.ToolsImageOverride = true
	out = systemImageHints("2.0.0", "docklet-next-agent:2.0.0", info, "2.0.0", "2.0.0")
	require.Equal(t, "pinned", out.Images[1].State)
	require.Empty(t, out.Images[1].CandidateVersion)
	info.ToolsImageOverride = false
	info.CurrentVersion = "1.9.0"
	info.CurrentImageRef = "docklet-next-agent:1.9.0"
	out = systemImageHints("2.0.0", "docklet-next-agent:2.0.0", info, "2.0.0", "2.0.0")
	require.Equal(t, "unknown", out.Images[1].State)
	require.Empty(t, out.Images[1].CompatibleReference)
}
