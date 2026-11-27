// Source: docklet-next/issues/680
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age implementation.

package updater

import (
	a "github.com/florianfinn/docklet-next/types/activity"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMinimumAgeTerminalMapping(t *testing.T) {
	for _, code := range []string{"update.minimum-age", "update.candidate-failed"} {
		s := a.RuntimeSnapshot{Activity: a.Activity{Status: a.StatusFailed, ImageLifecycle: &dto.LifecycleResult{Code: code}}, RuntimeKnowledge: a.RuntimeKnowledge{OutcomeState: a.RuntimeOutcomeKnown}}
		out, uncertain := imagePolicySnapshotTarget("stack", "run", s)
		require.False(t, uncertain)
		require.Equal(t, st.Skipped, out.Status)
		require.Equal(t, code, out.Code)
	}
}
