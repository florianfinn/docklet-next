// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original checked recovery target mapping table. M24 recovery evidence, validation and reason contracts.

package updater

import (
	a "github.com/florianfinn/docklet-next/types/activity"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPolicyRecoveryMapping(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		status                  a.Status
		outcome, code, wantCode string
		want                    st.RunStatus
		missing, unknown        bool
	}{
		{name: "success", status: a.StatusSuccess, want: st.Succeeded},
		{name: "restored", status: a.StatusFailed, outcome: "restored", code: "update.unstable", wantCode: "update.auto-restored", want: st.Failed},
		{name: "restore-failed", status: a.StatusFailed, outcome: "restore-failed", code: "update.unstable", wantCode: "update.auto-restore-failed", want: st.Failed},
		{name: "unavailable", status: a.StatusFailed, outcome: "unavailable", code: "update.unstable", wantCode: "update.auto-restore-unavailable", want: st.Failed},
		{name: "not-needed", status: a.StatusFailed, outcome: "not-needed", code: "image.busy", wantCode: "image.busy", want: st.Skipped},
		{name: "interrupted", status: a.StatusFailed, outcome: "interrupted", wantCode: "update.interrupted", want: st.Failed},
		{name: "after-effect-cancel", status: a.StatusCancelled, outcome: "interrupted", wantCode: "update.interrupted", want: st.Failed},
		{name: "no-effect-cancel", status: a.StatusCancelled, outcome: "not-needed", want: st.Canceled},
		{name: "queued-cancel", status: a.StatusCancelled, missing: true, want: st.Canceled},
		{name: "candidate-failed", status: a.StatusFailed, outcome: "not-needed", code: "update.candidate-failed", wantCode: "update.candidate-failed", want: st.Skipped},
		{name: "missing", status: a.StatusSuccess, missing: true, wantCode: "runtime.outcome-unknown", want: st.Failed, unknown: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := a.RuntimeSnapshot{Activity: a.Activity{Status: tc.status}, RuntimeKnowledge: a.RuntimeKnowledge{OutcomeState: a.RuntimeOutcomeKnown}}
			if !tc.missing {
				snapshot.ImageLifecycle = &dto.LifecycleResult{Code: tc.code}
				if tc.outcome != "" {
					snapshot.ImageLifecycle.Recovery = &dto.RecoveryResult{Outcome: tc.outcome}
				}
			}
			got, unknown := imagePolicySnapshotTarget("target", "run", snapshot)
			require.Equal(t, tc.unknown, unknown)
			require.Equal(t, tc.want, got.Status)
			require.Equal(t, tc.wantCode, got.Code)
		})
	}
}
