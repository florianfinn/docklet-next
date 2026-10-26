// Source: docklet-next/docs/design/update-policies.md
// Revision: 4023414fcf620a9ea4be2429ee70fa8fd8230f9c
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Policy execution guard sweep uses receipts, errors and persisted checkpoints.

package updater

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	a "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

func TestPolicyPreparedOrRefusedDuplicateNeverAdmits(t *testing.T) {
	for _, status := range []string{"prepared", "refused"} {
		t.Run(status, func(t *testing.T) {
			f := newPolicyFixture(t)
			p := f.save(t, "stack", "apply")
			child, _, err := f.p.store.PrepareChild(t.Context(), p, "same-wrapper")
			require.NoError(t, err)
			if status == "refused" {
				require.NoError(t, f.db.Model(&policy.Child{}).Where("id = ?", child.ID).Updates(map[string]any{"status": "refused", "code": "image.busy"}).Error)
			}
			out := f.run(t, "auto-update", st.Run{ID: "same-wrapper", AttemptCount: 1})
			require.Len(t, out.Targets, 1)
			require.Equal(t, st.Skipped, out.Targets[0].Status)
			require.Zero(t, f.admissions)
			if status == "prepared" {
				require.Equal(t, "runtime.interrupted", out.Targets[0].Code)
			} else {
				require.Equal(t, "image.busy", out.Targets[0].Code)
				require.Zero(t, f.snapshots)
			}
		})
	}
}

func TestPolicySnapshotDispositionMapping(t *testing.T) {
	for _, row := range []struct {
		name      string
		status    a.Status
		knowledge a.RuntimeOutcomeState
		expected  st.RunStatus
		uncertain bool
	}{
		{"success", a.StatusSuccess, a.RuntimeOutcomeKnown, st.Succeeded, false},
		{"failed", a.StatusFailed, a.RuntimeOutcomeKnown, st.Failed, false},
		{"cancelled", a.StatusCancelled, a.RuntimeOutcomeKnown, st.Canceled, false},
		{"known-running", a.StatusRunning, a.RuntimeOutcomeKnown, st.Failed, true},
		{"unknown-success", a.StatusSuccess, a.RuntimeOutcomeUnknown, st.Failed, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			target, unknown := imagePolicySnapshotTarget("stack", "child", a.RuntimeSnapshot{Activity: a.Activity{Status: row.status}, RuntimeKnowledge: a.RuntimeKnowledge{OutcomeState: row.knowledge}})
			require.Equal(t, row.expected, target.Status)
			require.Equal(t, row.uncertain, unknown)
			require.Equal(t, "child", target.ActivityID)
			if row.uncertain {
				require.Equal(t, "runtime.outcome-unknown", target.Code)
			}
		})
	}
}

func TestPolicyHostStopsOnProgressFailureOrCancellation(t *testing.T) {
	for _, boundary := range []string{"progress", "cancelled"} {
		t.Run(boundary, func(t *testing.T) {
			f := newPolicyFixture(t)
			f.save(t, "a-stack", "apply")
			f.save(t, "b-stack", "apply")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			failure := errors.New("synthetic progress write failure")
			var persist func(st.TargetOutcome) error
			if boundary == "progress" {
				persist = func(st.TargetOutcome) error { return failure }
			} else {
				persist = func(st.TargetOutcome) error { cancel(); return nil }
			}
			out, err := f.s.RunImagePolicyHost(jobcontext.WithExecution(ctx, st.Run{ID: "wrapper", AttemptCount: 1}, persist), "0", "auto-update")
			require.Error(t, err)
			require.Len(t, out.Targets, 1)
			require.Equal(t, 1, f.admissions)
			require.Equal(t, 1, f.checks)
		})
	}
}

func TestPolicyReadAndWriteFailuresAreVisible(t *testing.T) {
	for _, boundary := range []string{"host-error", "registration", "baseline-error", "check-error", "nil-check", "unknown-check", "error-check", "not-pulled-check", "prepare-write", "refusal-write", "policy-status-write", "candidate-write", "candidate-status-write", "notice-claim", "notice-read", "notice-settle"} {
		t.Run(boundary, func(t *testing.T) {
			f := newPolicyFixture(t)
			mode := "apply"
			job := "auto-update"
			if boundary == "candidate-write" || boundary == "candidate-status-write" || boundary == "notice-claim" || boundary == "notice-read" || boundary == "notice-settle" {
				mode = "notify"
				job = "image-polling"
			}
			f.save(t, "stack", mode)
			expected := "update.persistence-failed"
			switch boundary {
			case "host-error":
				expected = "update.target-changed"
				calls := 0
				f.p.ports.host = func(context.Context, string) (string, error) {
					calls++
					if calls > 1 {
						return "example-registration", errors.New("synthetic host read failure")
					}
					return "example-registration", nil
				}
			case "registration":
				expected = "update.target-changed"
				calls := 0
				f.p.ports.host = func(context.Context, string) (string, error) {
					calls++
					if calls == 1 {
						return "example-registration", nil
					}
					return "replacement", nil
				}
			case "baseline-error":
				expected = "runtime.target-unavailable"
				f.p.ports.baseline = func(context.Context, string, string) (dto.BaselineStatus, error) {
					return dto.BaselineStatus{}, errors.New("synthetic baseline read error")
				}
			case "check-error":
				expected = "update.check-unavailable"
				f.p.ports.check = func(context.Context, string, string) (*projectDTO.UpdateInfo, error) {
					return f.check, errors.New("synthetic check read error")
				}
			case "nil-check":
				expected = "update.check-unavailable"
				f.p.ports.check = func(context.Context, string, string) (*projectDTO.UpdateInfo, error) { return nil, nil }
			case "unknown-check":
				expected = "update.check-unavailable"
				f.check.Status = "unknown"
			case "error-check":
				expected = "update.check-unavailable"
				f.check.Status = "error"
			case "not-pulled-check":
				expected = "update.check-unavailable"
				f.check.Status = "not_pulled"
			case "prepare-write":
				require.NoError(t, f.db.Exec("CREATE TRIGGER synthetic_failure BEFORE INSERT ON image_policy_children BEGIN SELECT RAISE(ABORT,'synthetic prepare failure'); END").Error)
			case "refusal-write":
				f.p.ports.admit = func(context.Context, string, string, string, dto.LifecycleRequest, dto.PolicyBinding) (activity.RuntimeAdmissionResult, error) {
					return activity.RuntimeAdmissionResult{}, imageDTO.Failure("image.busy", 409)
				}
				require.NoError(t, f.db.Exec("CREATE TRIGGER synthetic_failure BEFORE UPDATE ON image_policy_children WHEN NEW.status='refused' BEGIN SELECT RAISE(ABORT,'synthetic refusal failure'); END").Error)
			case "policy-status-write":
				f.baseline.Available = false
				require.NoError(t, f.db.Exec("CREATE TRIGGER synthetic_failure BEFORE UPDATE ON stack_update_policies WHEN NEW.apply_code != OLD.apply_code BEGIN SELECT RAISE(ABORT,'synthetic status failure'); END").Error)
			case "candidate-write":
				require.NoError(t, f.db.Exec("CREATE TRIGGER synthetic_failure BEFORE INSERT ON image_update_candidates BEGIN SELECT RAISE(ABORT,'synthetic candidate failure'); END").Error)
			case "candidate-status-write":
				require.NoError(t, f.db.Exec("CREATE TRIGGER synthetic_failure BEFORE UPDATE ON stack_update_policies WHEN NEW.candidates != OLD.candidates BEGIN SELECT RAISE(ABORT,'synthetic candidate status failure'); END").Error)
			case "notice-claim":
				require.NoError(t, f.db.Exec("CREATE TRIGGER synthetic_failure BEFORE UPDATE ON image_update_notifications WHEN NEW.status='sending' BEGIN SELECT RAISE(ABORT,'synthetic claim failure'); END").Error)
			case "notice-read":
				require.NoError(t, f.db.Exec("CREATE TRIGGER synthetic_failure AFTER INSERT ON image_update_notifications BEGIN DELETE FROM image_update_notifications WHERE id=NEW.id; END").Error)
			case "notice-settle":
				require.NoError(t, f.db.Exec("CREATE TRIGGER synthetic_failure BEFORE UPDATE ON image_update_notifications WHEN NEW.status='sent' BEGIN SELECT RAISE(ABORT,'synthetic delivery settlement failure'); END").Error)
			}
			out := f.run(t, job, st.Run{})
			require.Len(t, out.Targets, 1)
			require.Equal(t, expected, out.Targets[0].Code)
			require.Zero(t, f.admissions)
			if boundary == "notice-settle" {
				require.Equal(t, 1, f.notifications)
				f.run(t, job, st.Run{})
				require.Equal(t, 1, f.notifications, "an interrupted sending claim cannot resend")
				row, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
				require.NoError(t, err)
				require.Len(t, row.Candidates, 1)
				require.Equal(t, "sending", row.Candidates[0].NotificationState)
			}
		})
	}
}

func TestPolicyCandidateWriteCASBeforeCheckpoint(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "stack", "apply")
	original := f.p.ports.actor
	f.p.ports.actor = func(ctx context.Context, env, actor, key string, apply bool) (context.Context, error) {
		result, err := original(ctx, env, actor, key, apply)
		if f.actors == 2 {
			f.save(t, "stack", "apply")
		}
		return result, err
	}
	out := f.run(t, "auto-update", st.Run{})
	require.Equal(t, "update.policy-changed", out.Targets[0].Code)
	require.Zero(t, f.admissions)
	var count int64
	require.NoError(t, f.db.Model(&policy.Child{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestPolicyReconcileUsesRecordedRuntimeIDAndNoReplay(t *testing.T) {
	f := newPolicyFixture(t)
	p := f.save(t, "stack", "apply")
	child, _, err := f.p.store.PrepareChild(t.Context(), p, "wrapper")
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&policy.Child{}).Where("id = ?", child.ID).Update("status", "admitted").Error)
	f.receipts[child.ID] = a.RuntimeSnapshot{Activity: a.Activity{Status: a.StatusCancelled}, RuntimeKnowledge: a.RuntimeKnowledge{OutcomeState: a.RuntimeOutcomeKnown}}
	out := f.run(t, "auto-update", st.Run{ID: "wrapper", AttemptCount: 2, CreatedAt: time.Now()})
	require.Len(t, out.Targets, 1)
	require.Equal(t, st.Canceled, out.Targets[0].Status)
	require.Zero(t, f.admissions)
}

func TestPolicyReasonFallbackRejectsUnknownDynamicCodes(t *testing.T) {
	for _, err := range []error{imageDTO.Failure("synthetic.unknown", 500), &activity.GateConflict{Code: "synthetic.unknown"}} {
		require.Equal(t, "update.persistence-failed", policyCode(err, "update.persistence-failed"))
	}
}

func TestPolicyManualTriggerUsesItsCurrentActor(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "stack", "apply")
	f.p.ports.scheduleActor = func(_ context.Context, env, actor, key string) error {
		if actor != "manual-operator" || key != "manual-key" {
			return errors.New("synthetic wrong trigger owner")
		}
		return nil
	}
	out := f.run(t, "auto-update", st.Run{RequestedBy: "manual-operator", RequestedWithKey: "manual-key"})
	require.Equal(t, st.Succeeded, out.Status)
	require.Equal(t, 1, f.admissions)
}

func TestPolicyNoCandidateDoesNotAdmit(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "stack", "apply")
	f.check.HasUpdate = false
	f.check.ServiceUpdates["app"].UpdateInfo.HasUpdate = false
	out := f.run(t, "auto-update", st.Run{})
	require.Equal(t, st.Succeeded, out.Targets[0].Status)
	require.Zero(t, f.admissions)
}

func TestPolicyReconcileReadFailureNeverDispatches(t *testing.T) {
	f := newPolicyFixture(t)
	p := f.save(t, "stack", "apply")
	child, _, err := f.p.store.PrepareChild(t.Context(), p, "wrapper")
	require.NoError(t, err)
	require.NoError(t, policy.AdmitChild(f.db.DB, &dto.PolicyBinding{ChildID: child.ID}, child.ID))
	f.p.ports.snapshot = func(context.Context, string, string) (a.RuntimeSnapshot, bool, error) {
		return a.RuntimeSnapshot{Activity: a.Activity{Status: a.StatusSuccess}, RuntimeKnowledge: a.RuntimeKnowledge{OutcomeState: a.RuntimeOutcomeKnown}}, true, errors.New("synthetic receipt read failure")
	}
	out := f.run(t, "auto-update", st.Run{ID: "wrapper", AttemptCount: 2})
	require.Len(t, out.Targets, 1)
	require.Equal(t, "runtime.outcome-unknown", out.Targets[0].Code)
	require.Zero(t, f.admissions)
}

func TestPolicyListIsHostScoped(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "a-stack", "apply")
	other := f.save(t, "b-stack", "apply")
	require.NoError(t, f.db.Delete(&policy.Policy{}, "id = ?", other.ID).Error)
	other.EnvironmentID = "other-host"
	other.ID = policy.Key(other.EnvironmentID, other.RegistrationID, other.TargetID)
	require.NoError(t, f.p.store.SavePolicy(t.Context(), other, 0))
	out := f.run(t, "auto-update", st.Run{})
	require.Len(t, out.Targets, 1)
	require.Equal(t, "a-stack", out.Targets[0].ID)
	require.Equal(t, 1, f.admissions)
}

func TestPolicyApplyStatusCannotOverwriteNewRevision(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "stack", "apply")
	f.afterAdmission = func(dto.PolicyBinding) { f.save(t, "stack", "off") }
	out := f.run(t, "auto-update", st.Run{})
	require.Equal(t, st.Succeeded, out.Targets[0].Status)
	row, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
	require.NoError(t, err)
	require.Equal(t, "off", row.Mode)
	require.Nil(t, row.ApplyCheckedAt)
}

func TestPolicyCandidateRequiresPositiveComparableState(t *testing.T) {
	for _, state := range []string{"current", "unknown", "error", "missing"} {
		t.Run(state, func(t *testing.T) {
			f := newPolicyFixture(t)
			f.save(t, "stack", "apply")
			row := f.check.ServiceUpdates["app"]
			if state == "missing" {
				row.UpdateInfo = nil
			} else {
				row.UpdateInfo.State = state
			}
			f.check.ServiceUpdates["app"] = row
			out := f.run(t, "auto-update", st.Run{})
			require.Equal(t, st.Succeeded, out.Targets[0].Status)
			require.Zero(t, f.admissions)
		})
	}
}

func TestPolicyCandidateVersionNormalizationDeduplicates(t *testing.T) {
	for _, source := range []string{"explicit", "tag-fallback"} {
		t.Run(source, func(t *testing.T) {
			f := newPolicyFixture(t)
			f.save(t, "stack", "notify")
			row := f.check.ServiceUpdates["app"]
			row.ImageRef = "registry.example.com/app:v1.0.0"
			row.UpdateInfo.State = "rebuild"
			row.UpdateInfo.LatestVersion = "v1.0.0"
			if source == "tag-fallback" {
				row.UpdateInfo.LatestVersion = ""
			}
			f.check.ServiceUpdates["app"] = row
			f.run(t, "image-polling", st.Run{})
			require.Equal(t, 1, f.notifications)
			first, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
			require.NoError(t, err)
			require.Len(t, first.Candidates, 1)
			require.Equal(t, "1.0.0", first.Candidates[0].Version)
			row.UpdateInfo.LatestVersion = "1.0.0"
			f.run(t, "image-polling", st.Run{})
			require.Equal(t, 1, f.notifications)
			second, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
			require.NoError(t, err)
			require.Equal(t, first.Candidates[0].EntryID, second.Candidates[0].EntryID)
		})
	}
}
