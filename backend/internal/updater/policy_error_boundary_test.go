// Source: docklet-next/docs/design/update-policies.md
// Revision: 4023414fcf620a9ea4be2429ee70fa8fd8230f9c
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Explicit execution-input, storage-error and recovery-projection barriers.

package updater

import (
	"context"
	"errors"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	a "github.com/florianfinn/docklet-next/types/activity"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

func TestPolicyExecutionRequiresValidOwnedContext(t *testing.T) {
	for _, boundary := range []string{"missing-context", "empty-id", "unknown-job", "agent"} {
		t.Run(boundary, func(t *testing.T) {
			f := newPolicyFixture(t)
			f.save(t, "stack", "apply")
			ctx := t.Context()
			job := "auto-update"
			run := st.Run{ID: "wrapper", AttemptCount: 1}
			if boundary == "empty-id" {
				run.ID = ""
			}
			if boundary == "unknown-job" {
				job = "unsupported"
			}
			if boundary != "missing-context" {
				ctx = jobcontext.WithExecution(ctx, run, nil)
			}
			expected := "update.invalid-target"
			if boundary == "agent" {
				f.s.config = &config.Config{AgentMode: true}
				expected = "update.agent-local-job-disabled"
			}
			out, err := f.s.RunImagePolicyHost(ctx, "0", job)
			require.NoError(t, err)
			require.Equal(t, expected, out.Code)
			require.Zero(t, f.checks)
			require.Zero(t, f.admissions)
		})
	}
}

func TestPolicyStorageErrorBoundaries(t *testing.T) {
	for _, boundary := range []string{"schedule-read", "host-hold-read", "policy-list-read", "host-hold-settle", "recovery-children", "recovery-policy"} {
		t.Run(boundary, func(t *testing.T) {
			f := newPolicyFixture(t)
			p := f.save(t, "stack", "apply")
			job := "auto-update"
			run := st.Run{}
			expected := "update.persistence-failed"
			switch boundary {
			case "schedule-read":
				expected = "runtime.target-unavailable"
				f.p.ports.host = func(context.Context, string) (string, error) {
					return "example-registration", errors.New("synthetic schedule read failure")
				}
			case "host-hold-read":
				require.NoError(t, f.db.Exec("DROP TABLE image_policy_children").Error)
			case "policy-list-read":
				job = "image-polling"
				require.NoError(t, f.db.Exec("DROP TABLE stack_update_policies").Error)
			case "host-hold-settle":
				require.NoError(t, f.db.Exec("CREATE TRIGGER synthetic_hold_failure BEFORE UPDATE ON image_policy_children WHEN NEW.status='settled' BEGIN SELECT RAISE(ABORT,'synthetic hold checkpoint failure'); END").Error)
				f.run(t, job, st.Run{})
				require.Equal(t, 1, f.admissions)
			case "recovery-children":
				run.Trigger = "recovery"
				require.NoError(t, f.db.Exec("DROP TABLE image_policy_children").Error)
			case "recovery-policy":
				run = st.Run{ID: "wrapper", Trigger: "recovery"}
				_, _, err := f.p.store.PrepareChild(t.Context(), p, run.ID)
				require.NoError(t, err)
				require.NoError(t, f.db.Delete(&policy.Policy{}, "id = ?", p.ID).Error)
				expected = "runtime.interrupted"
			}
			before := f.admissions
			out := f.run(t, job, run)
			require.Equal(t, expected, out.Code)
			require.Equal(t, st.Failed, out.Status)
			require.Equal(t, before, f.admissions)
			if boundary == "recovery-policy" {
				require.Empty(t, out.Targets)
				require.Zero(t, f.snapshots)
			}
		})
	}
}

func TestPolicyAggregateDisposition(t *testing.T) {
	require.Equal(t, st.Partial, policyResult(st.Outcome{Status: st.Succeeded, Targets: []st.TargetOutcome{{Status: st.Failed}}}).Status)
	require.Equal(t, st.Failed, policyResult(st.Outcome{Status: st.Failed, Targets: []st.TargetOutcome{{Status: st.Failed}}}).Status)
	require.Equal(t, st.Succeeded, policyResult(st.Outcome{Status: st.Succeeded, Targets: []st.TargetOutcome{{Status: st.Succeeded}}}).Status)
}

func TestPolicyRecoveryMergesRecordedTargets(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "append", true: "replace"}[existing], func(t *testing.T) {
			f := newPolicyFixture(t)
			p := f.save(t, "stack", "apply")
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
