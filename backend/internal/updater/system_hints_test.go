// Source: docklet-next/docs/design/images-updates.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Pure release-evidence projection refuses above-hub, ambiguous and invented candidates; product feed states (#235).

package updater

import (
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	versionDTO "github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSystemImageHintsAreManualAndCapped(t *testing.T) {
	for _, test := range []struct {
		name, hub, agent, latest, best, code, reference, candidate, state, wantCode string
		requires                                                                bool
	}{
		{"capped", "2.0.0", "1.9.0", "2.1.0", "2.0.0", "", "docklet-next-agent:1.9.0", "2.0.0", "available", "", true},
		{"no release at or below hub is current", "2.0.0", "1.9.0", "2.1.0", "", "", "docklet-next-agent:1.9.0", "", "current", "", true},
		{"empty feed is current", "2.0.0", "2.0.0", "", "", "", "docklet-next-agent:2.0.0", "", "current", "", false},
		{"agent newer", "2.0.0", "2.1.0", "2.2.0", "2.0.0", "", "docklet-next-agent:2.1.0", "", "unknown", "update.requires-hub-update", true},
		{"numeric ordering", "2.10.0", "2.9.0", "2.10.0", "2.10.0", "", "docklet-next-agent:2.9.0", "2.10.0", "available", "", false},
		{"rc agent below stable", "2.0.0", "2.0.0-rc.1", "2.1.0", "2.0.0", "", "docklet-next-agent:2.0.0-rc.1", "2.0.0", "available", "", true},
		{"rc hub keeps older stable agent current", "1.0.0-rc.2", "1.0.0-rc.2", "0.9.0", "0.9.0", "", "docklet-next-agent:1.0.0-rc.2", "", "current", "", false},
		{"custom reference", "2.0.0", "1.9.0", "2.1.0", "2.0.0", "", "registry.example.com/custom:1.9.0", "", "unknown", "update.system-version-unknown", false},
		{"untrusted above cap", "2.0.0", "1.9.0", "2.1.0", "2.1.0", "", "docklet-next-agent:1.9.0", "", "current", "", true},
		{"no downgrade", "2.0.0", "1.9.1", "2.1.0", "1.9.0", "", "docklet-next-agent:1.9.1", "", "current", "", true},
		{"feed disabled", "2.0.0", "1.9.0", "", "", "feature-disabled", "docklet-next-agent:1.9.0", "", "unknown", "feature-disabled", false},
		{"feed error", "2.0.0", "1.9.0", "", "", "update.check-unavailable", "docklet-next-agent:1.9.0", "", "unknown", "update.check-unavailable", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			feed := releaseFeedObservation{latest: test.latest, best: test.best, code: test.code}
			out := systemImageHints(test.hub, "docklet-next-agent:"+test.hub, versionDTO.Info{CurrentVersion: test.agent, CurrentImageRef: test.reference}, feed)
			require.Len(t, out.Images, 2)
			agent := out.Images[0]
			require.True(t, agent.ManualOnly)
			require.Equal(t, test.candidate, agent.CandidateVersion)
			require.Equal(t, test.requires, agent.RequiresHubUpdate)
			require.Equal(t, test.state, agent.State)
			require.Equal(t, test.wantCode, agent.Code)
		})
	}
}
func TestSystemImageHintsCustomConfiguredReferenceRemainsUnknown(t *testing.T) {
	reference := "registry.example.com/custom-agent:1.9.0"
	out := systemImageHints("2.0.0", reference, versionDTO.Info{CurrentVersion: "1.9.0", CurrentImageRef: reference}, releaseFeedObservation{latest: "2.1.0", best: "2.0.0"})
	require.Equal(t, "unknown", out.Images[0].State)
	require.Empty(t, out.Images[0].CandidateVersion)
}

func TestSystemToolsHintPreservesPinsAndReleaseCompatibility(t *testing.T) {
	info := versionDTO.Info{CurrentVersion: "2.0.0", CurrentImageRef: "docklet-next-agent:2.0.0", ToolsImage: volumehelper.DefaultToolsImage}
	out := systemImageHints("2.0.0", "docklet-next-agent:2.0.0", info, releaseFeedObservation{latest: "2.0.0", best: "2.0.0"})
	require.Equal(t, "current", out.Images[1].State)
	require.Equal(t, volumehelper.DefaultToolsImage, out.Images[1].CompatibleReference)
	info.ToolsImageOverride = true
	out = systemImageHints("2.0.0", "docklet-next-agent:2.0.0", info, releaseFeedObservation{latest: "2.0.0", best: "2.0.0"})
	require.Equal(t, "pinned", out.Images[1].State)
	require.Empty(t, out.Images[1].CandidateVersion)
	info.ToolsImageOverride = false
	info.CurrentVersion = "1.9.0"
	info.CurrentImageRef = "docklet-next-agent:1.9.0"
	out = systemImageHints("2.0.0", "docklet-next-agent:2.0.0", info, releaseFeedObservation{latest: "2.0.0", best: "2.0.0"})
	require.Equal(t, "unknown", out.Images[1].State)
	require.Empty(t, out.Images[1].CompatibleReference)
}
