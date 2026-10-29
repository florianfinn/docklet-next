// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package job

import (
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAutoHealScheduledRunHasNoEmptyActivity(t *testing.T) {
	s, _, _ := newJobActivityTestServiceInternal(t)
	for _, trigger := range []string{"scheduled", "recovery", "manual"} {
		r, e := s.runs.Submit(t.Context(), scheduler.Request{JobID: "auto-heal", EnvironmentID: "0", Trigger: trigger})
		require.NoError(t, e)
		require.Equal(t, trigger == "manual", r.ActivityID != "")
	}
}
