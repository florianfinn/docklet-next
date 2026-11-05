// Source: docklet-next/docs/design/update-policies.md
// Revision: 4023414fcf620a9ea4be2429ee70fa8fd8230f9c
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Explicit execution-input, storage-error and recovery-projection barriers.

package updater

import (
	"testing"

	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	a "github.com/florianfinn/docklet-next/types/activity"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

func TestPolicyAggregateDisposition(t *testing.T) {
	require.Equal(t, st.Partial, policyResult(st.Outcome{Status: st.Succeeded, Targets: []st.TargetOutcome{{Status: st.Failed}}}).Status)
	require.Equal(t, st.Failed, policyResult(st.Outcome{Status: st.Failed, Targets: []st.TargetOutcome{{Status: st.Failed}}}).Status)
	require.Equal(t, st.Succeeded, policyResult(st.Outcome{Status: st.Succeeded, Targets: []st.TargetOutcome{{Status: st.Succeeded}}}).Status)
}

func TestPolicyRecoveryMergesRecordedTargets(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "append", true: "replace"}[existing], func(t *testing.T) {
			f := newPolicyFixture(t)
			p := f.save(t, "stack", "scheduled")
			child, _, err := f.p.store.PrepareChild(t.Context(), p, "wrapper")
			require.NoError(t, err)
			require.NoError(t, policy.AdmitChild(f.db.DB, &dto.PolicyBinding{ChildID: child.ID}, child.ID))
			f.receipts[child.ID] = a.RuntimeSnapshot{Activity: a.Activity{Status: a.StatusSuccess}, RuntimeKnowledge: a.RuntimeKnowledge{OutcomeState: a.RuntimeOutcomeKnown}}
			run := st.Run{ID: "wrapper", AttemptCount: 2, Outcome: st.Outcome{Targets: []st.TargetOutcome{{ID: "unrelated", Status: st.Skipped}}}}
			if existing {
				run.Outcome.Targets = append(run.Outcome.Targets, st.TargetOutcome{ID: "stack", Status: st.Failed})
			}
			out := f.run(t, "auto-update", run)
			require.Len(t, out.Targets, 2)
			require.Equal(t, "unrelated", out.Targets[0].ID)
			require.Equal(t, st.Skipped, out.Targets[0].Status)
			require.Equal(t, "stack", out.Targets[1].ID)
			require.Equal(t, st.Succeeded, out.Targets[1].Status)
			require.Zero(t, f.admissions)
		})
	}
}
