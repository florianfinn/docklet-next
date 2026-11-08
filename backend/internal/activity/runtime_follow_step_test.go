// Source: docklet-next/issues/173
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original unit proof of the follower stop and departure recheck decision.

package activity

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
)

func TestRuntimeFollowStepDecidesKeepAndDepartureRecheck(t *testing.T) {
	snapshot := func(status activitytypes.Status, outcome activitytypes.RuntimeOutcomeState) activitytypes.RuntimeSnapshot {
		return activitytypes.RuntimeSnapshot{
			Activity:         activitytypes.Activity{ID: "run", Status: status},
			RuntimeKnowledge: activitytypes.RuntimeKnowledge{OutcomeState: outcome},
		}
	}
	running := snapshot(activitytypes.StatusRunning, activitytypes.RuntimeOutcomeKnown)
	cases := []struct {
		name          string
		snapshot      activitytypes.RuntimeSnapshot
		err           error
		keep, recheck bool
	}{
		{"known active keeps following", running, nil, true, false},
		{"terminal departs with recheck", snapshot(activitytypes.StatusSuccess, activitytypes.RuntimeOutcomeKnown), nil, false, true},
		{"unknown outcome departs with recheck", snapshot(activitytypes.StatusRunning, activitytypes.RuntimeOutcomeUnknown), nil, false, true},
		{"recovery in progress keeps following", running, fmt.Errorf("refresh: %w", ErrRuntimeRecoveryInProgress), true, false},
		{"refresh error departs without recheck", running, errors.New("agent unavailable"), false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keep, recheck := runtimeFollowStepInternal(tc.snapshot, tc.err)
			require.Equal(t, tc.keep, keep, "keep")
			require.Equal(t, tc.recheck, recheck, "recheck")
		})
	}
}
