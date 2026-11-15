// Source: docklet-next/issues/366
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original narrow refused gate-code projection and interrupted controls.

package activity

import (
	a "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRefusedGateProjection(t *testing.T) {
	for _, tc := range []struct{ disposition, reason, code string }{
		{"refused", "run.busy", "run.busy"},
		{"refused", "run.cancelling", "run.cancelling"},
		{"refused", "runtime.target-changed", "runtime.interrupted"},
		{"interrupted", "run.busy", "runtime.interrupted"},
		{"released", "run.busy", "runtime.interrupted"},
	} {
		t.Run(tc.disposition+"/"+tc.reason, func(t *testing.T) {
			row := RuntimeReceipt{EnvironmentID: "example-remote", DispatchState: new("pending"), ExecutorState: new("pending"), OperationID: new("stop-container"), HubDisposition: &tc.disposition, HubDispositionReason: &tc.reason}
			model := Activity{}
			runtimeProjectModelInternal(&model, &row)
			require.Equal(t, tc.code, model.MessageCode)
			if tc.disposition == "refused" {
				require.Equal(t, a.RuntimeOutcomeKnown, model.runtimeKnowledge.OutcomeState)
			}
		})
	}
}
