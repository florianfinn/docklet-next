// Source: docklet-next/issues/330
// Revision: 20a03b67fbeb6bcc82c758e6f2ece937d58f037a
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original regression for Docklet's own hub and agent labels in the bind self rule (M16 review CA-1).

package bindpath

import (
	"os"
	"regexp"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

// The agent template and both images label Docklet's containers; R2 refuses their mounts on every host.
func TestBindRefusesDockletContainersByTheirOwnLabels(t *testing.T) {
	agent := binding("agent", "/opt/docklet-agent/data", map[string]string{"io.docklet.system": "agent", "com.getarcaneapp.arcane.hidden": "true"})
	hub := binding("hub", "/srv/docklet-hub/data", map[string]string{"io.docklet.system": "hub"})
	for _, tc := range []struct {
		source string
		owner  container.Summary
	}{{"/opt/docklet-agent/data", agent}, {"/opt/docklet-agent", agent}, {"/srv/docklet-hub/data/backups", hub}} {
		var p *dto.Problem
		require.ErrorAs(t, Rules(tc.source, "", []container.Summary{tc.owner}, nil, true), &p, tc.source)
		require.Equal(t, "docklet", p.Reason, tc.source)
	}
	require.False(t, Helper(agent))
	require.False(t, Self(binding("other", "/srv/app", map[string]string{"io.docklet.system": "workload"})))
}

func TestDockletImagesCarryTheSelfLabel(t *testing.T) {
	dockerfile, err := os.ReadFile("../../../../deploy/hub/Dockerfile")
	require.NoError(t, err)
	for _, stage := range []string{"agent", "hub"} {
		stageLabel := regexp.MustCompile(`(?m)^FROM \S+ AS ` + stage + `\nLABEL io\.docklet\.system=` + stage + `\n`)
		require.True(t, stageLabel.Match(dockerfile), "%s image must carry io.docklet.system=%s", stage, stage)
		require.True(t, Self(container.Summary{Labels: map[string]string{"io.docklet.system": stage}}))
	}
}
