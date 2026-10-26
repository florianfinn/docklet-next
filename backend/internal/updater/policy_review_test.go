// Source: docklet-next/docs/design/update-policies.md
// Revision: 4023414fcf620a9ea4be2429ee70fa8fd8230f9c
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Lost-response barriers, independent job status and optional manual bookkeeping.

package updater

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	digest "github.com/opencontainers/go-digest"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	a "github.com/florianfinn/docklet-next/types/activity"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

func TestPolicyAdmissionErrorAndWaitErrorHoldSerialHost(t *testing.T) {
	for _, boundary := range []string{"committed-receipt", "receipt-unavailable", "wait-error", "mismatched-acceptance", "settle-write-failure"} {
		t.Run(boundary, func(t *testing.T) {
			f := newPolicyFixture(t)
			f.save(t, "a-stack", "apply")
			f.save(t, "b-stack", "apply")
			original := f.p.ports.admit
			f.afterAdmission = func(b dto.PolicyBinding) {
				f.receipts[b.ChildID] = a.RuntimeSnapshot{Activity: a.Activity{Status: a.StatusRunning}, RuntimeKnowledge: a.RuntimeKnowledge{OutcomeState: a.RuntimeOutcomeUnknown}}
			}
			if boundary == "committed-receipt" || boundary == "receipt-unavailable" {
				f.p.ports.admit = func(ctx context.Context, env, target, key string, r dto.LifecycleRequest, b dto.PolicyBinding) (activity.RuntimeAdmissionResult, error) {
					result, err := original(ctx, env, target, key, r, b)
					require.NoError(t, err)
					return result, errors.New("synthetic error after commit")
				}
			}
			if boundary == "receipt-unavailable" {
				f.p.ports.snapshot = func(context.Context, string, string) (a.RuntimeSnapshot, bool, error) {
					return a.RuntimeSnapshot{}, false, errors.New("synthetic unavailable receipt")
				}
			}
			if boundary == "wait-error" {
				f.wait = func(string) (a.RuntimeSnapshot, error) {
					return a.RuntimeSnapshot{}, errors.New("synthetic waiter disconnected")
				}
			}
			if boundary == "mismatched-acceptance" {
				f.p.ports.admit = func(ctx context.Context, env, target, key string, r dto.LifecycleRequest, b dto.PolicyBinding) (activity.RuntimeAdmissionResult, error) {
					result, err := original(ctx, env, target, key, r, b)
					result.ActivityID = "another-run"
					return result, err
				}
			}
			if boundary == "settle-write-failure" {
				f.afterAdmission = nil
				require.NoError(t, f.db.Exec("CREATE TRIGGER synthetic_settle_failure BEFORE UPDATE ON image_policy_children WHEN NEW.status='settled' BEGIN SELECT RAISE(ABORT,'synthetic settlement write failure'); END").Error)
			}
			out := f.run(t, "auto-update", st.Run{})
			require.Len(t, out.Targets, 1)
			require.Equal(t, st.Failed, out.Status)
			require.Equal(t, 1, f.admissions, "a second stack cannot start across an uncertain first child")
			if boundary == "mismatched-acceptance" {
				require.Zero(t, f.waits, "never follow an unrelated accepted run")
			}
			var children []policy.Child
			require.NoError(t, f.db.Find(&children).Error)
			require.Len(t, children, 1)
			require.Equal(t, "admitted", children[0].Status)
			if boundary != "settle-write-failure" {
				blocked := f.run(t, "auto-update", st.Run{})
				require.Equal(t, "runtime.outcome-unknown", blocked.Code)
				require.Equal(t, 1, f.admissions)
			}
		})
	}
}

func TestPolicyLateTerminalReceiptAllowsNextChild(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "a-stack", "apply")
	f.save(t, "b-stack", "apply")
	original := f.p.ports.admit
	f.p.ports.admit = func(ctx context.Context, env, target, key string, r dto.LifecycleRequest, b dto.PolicyBinding) (activity.RuntimeAdmissionResult, error) {
		result, err := original(ctx, env, target, key, r, b)
		require.NoError(t, err)
		return result, errors.New("synthetic lost terminal response")
	}
	out := f.run(t, "auto-update", st.Run{})
	require.Equal(t, 2, f.admissions)
	require.Len(t, out.Targets, 2)
	for _, target := range out.Targets {
		require.Equal(t, st.Succeeded, target.Status)
		require.Empty(t, target.Code)
	}
	require.Zero(t, f.waits)
}

func TestPolicyBusySettlementIsSkipped(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "a-stack", "apply")
	f.save(t, "b-stack", "apply")
	f.wait = func(id string) (a.RuntimeSnapshot, error) {
		snapshot := f.receipts[id]
		snapshot.Status = a.StatusFailed
		snapshot.ImageLifecycle = &dto.LifecycleResult{Code: "image.busy"}
		return snapshot, nil
	}
	out := f.run(t, "auto-update", st.Run{})
	require.Len(t, out.Targets, 2)
	require.Equal(t, 2, f.admissions)
	for _, target := range out.Targets {
		require.Equal(t, st.Skipped, target.Status)
		require.Equal(t, "image.busy", target.Code)
	}
}

func TestPolicyCheckAndApplyStatusAreIndependent(t *testing.T) {
	t.Run("notify check then apply skip", func(t *testing.T) {
		f := newPolicyFixture(t)
		f.save(t, "stack", "notify")
		f.run(t, "image-polling", st.Run{})
		before, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
		require.NoError(t, err)
		require.False(t, before.Stale)
		f.run(t, "auto-update", st.Run{})
		after, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
		require.NoError(t, err)
		require.Equal(t, before.CheckedAt, after.CheckedAt)
		require.Empty(t, after.Code)
		require.False(t, after.Stale)
		require.Nil(t, after.ApplyCheckedAt)
	})
	t.Run("drift refusal then poll", func(t *testing.T) {
		f := newPolicyFixture(t)
		f.save(t, "stack", "apply")
		f.baseline.Available = false
		f.baseline.Code = "update.config-drift"
		f.run(t, "auto-update", st.Run{})
		refused, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
		require.NoError(t, err)
		require.Equal(t, "update.config-drift", refused.ApplyCode)
		require.NotNil(t, refused.ApplyCheckedAt)
		f.run(t, "image-polling", st.Run{})
		polled, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
		require.NoError(t, err)
		require.Equal(t, refused.ApplyCode, polled.ApplyCode)
		require.Equal(t, refused.ApplyCheckedAt, polled.ApplyCheckedAt)
		require.False(t, polled.Stale)
		require.Empty(t, polled.Code)
		require.Len(t, polled.Candidates, 1)
		require.Zero(t, f.admissions)
	})
}

func TestManualCheckSucceedsDespitePolicyBookkeepingFailure(t *testing.T) {
	for _, boundary := range []string{"invalid-candidate", "revision-race", "entry-write", "status-write", "host-read", "policy-read"} {
		t.Run(boundary, func(t *testing.T) {
			f := newPolicyFixture(t)
			p := f.save(t, "stack", "notify")
			expected := "update.check-unavailable"
			switch boundary {
			case "invalid-candidate":
				f.check.ServiceUpdates["app"].UpdateInfo.LatestDigest = "invalid-digest"
			case "revision-race":
				expected = "update.policy-changed"
				f.p.ports.baseline = func(context.Context, string, string) (dto.BaselineStatus, error) {
					f.save(t, "stack", "notify")
					return f.baseline, nil
				}
			case "entry-write":
				expected = "update.persistence-failed"
				require.NoError(t, f.db.Exec("CREATE TRIGGER synthetic_entry_failure BEFORE INSERT ON image_update_notifications BEGIN SELECT RAISE(ABORT,'synthetic entry failure'); END").Error)
			case "status-write":
				expected = "update.persistence-failed"
				require.NoError(t, f.db.Exec("CREATE TRIGGER synthetic_bookkeeping_failure BEFORE UPDATE ON stack_update_policies WHEN NEW.bookkeeping_at IS NOT NULL BEGIN SELECT RAISE(ABORT,'synthetic bookkeeping failure'); END").Error)
			case "host-read":
				expected = "update.persistence-failed"
				f.p.ports.host = func(context.Context, string) (string, error) { return "", errors.New("synthetic host read failure") }
			case "policy-read":
				expected = "update.persistence-failed"
				require.NoError(t, f.db.Exec("DROP TABLE stack_update_policies").Error)
			}
			response, err := (&UpdaterHandler{updaterService: f.s}).CheckProjectUpdates(t.Context(), &dto.CheckProjectInput{EnvironmentID: "0", ProjectID: "stack"})
			var out *projectDTO.UpdateInfo
			if response != nil {
				require.True(t, response.Body.Success)
				out = response.Body.Data
			}
			require.NoError(t, err)
			require.NotNil(t, out)
			require.Equal(t, "has_update", out.Status)
			require.True(t, out.HasUpdate)
			require.Equal(t, expected, out.PolicyCode)
			if boundary == "invalid-candidate" || boundary == "entry-write" {
				var row policy.Policy
				require.NoError(t, f.db.First(&row, "id = ?", p.ID).Error)
				require.Empty(t, row.Code)
				require.NotNil(t, row.CheckedAt)
				require.Equal(t, expected, row.BookkeepingCode)
				require.NotNil(t, row.BookkeepingAt)
			}
			if boundary == "revision-race" {
				var current policy.Policy
				require.NoError(t, f.db.First(&current, "id = ?", p.ID).Error)
				require.EqualValues(t, 2, current.Revision)
				require.Nil(t, current.CheckedAt)
				require.Nil(t, current.BookkeepingAt)
			}
			require.Zero(t, f.admissions)
		})
	}
}

func TestPolicyEmissionReasonsAndPermissionStatus(t *testing.T) {
	for _, boundary := range []string{"before-check", "after-check", "before-admit"} {
		t.Run(boundary, func(t *testing.T) {
			f := newPolicyFixture(t)
			f.save(t, "stack", "apply")
			original := f.p.ports.actor
			f.p.ports.actor = func(ctx context.Context, env, actor, key string, apply bool) (context.Context, error) {
				limit := map[string]int{"before-check": 0, "after-check": 1, "before-admit": 2}[boundary]
				if f.actors >= limit {
					f.revoked = true
				}
				return original(ctx, env, actor, key, apply)
			}
			out := f.run(t, "auto-update", st.Run{})
			require.Len(t, out.Targets, 1)
			require.Equal(t, st.Failed, out.Targets[0].Status)
			require.Equal(t, "update.permission-denied", out.Targets[0].Code)
			require.Zero(t, f.admissions)
		})
	}
}

func TestPolicyEmptyScheduleOwnerHasDedicatedReason(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "stack", "apply")
	require.NoError(t, f.db.Model(&policy.HostSchedule{}).Where("environment_id = ?", "0").Update("actor_id", "").Error)
	out := f.run(t, "auto-update", st.Run{})
	require.Equal(t, "update.schedule-owner-unset", out.Code)
	require.Equal(t, st.Skipped, out.Status)
	require.Zero(t, f.checks)
}

func TestPolicyRejectsUncomparableObservationParts(t *testing.T) {
	for _, part := range []string{"reference", "platform", "platform-os", "platform-arch", "digest"} {
		t.Run(part, func(t *testing.T) {
			f := newPolicyFixture(t)
			f.save(t, "stack", "apply")
			one := f.check.ServiceUpdates["app"]
			switch part {
			case "reference":
				one.ImageRef = "invalid ref"
			case "platform":
				one.UpdateInfo.Platform = ""
			case "platform-os":
				one.UpdateInfo.Platform = "/amd64"
			case "platform-arch":
				one.UpdateInfo.Platform = "linux"
			case "digest":
				hash := sha512.Sum512([]byte("synthetic alternate digest"))
				one.UpdateInfo.LatestDigest = "sha512:" + hex.EncodeToString(hash[:])
				_, err := digest.Parse(one.UpdateInfo.LatestDigest)
				require.NoError(t, err)
			}
			f.check.ServiceUpdates["app"] = one
			out := f.run(t, "auto-update", st.Run{})
			require.Equal(t, "update.check-unavailable", out.Targets[0].Code)
			require.Zero(t, f.admissions)
		})
	}
}

func TestManualPolicyBookkeepingRecoversWithoutChangingApplyStatus(t *testing.T) {
	f := newPolicyFixture(t)
	p := f.save(t, "stack", "notify")
	require.NoError(t, f.db.Model(&policy.Policy{}).Where("id = ?", p.ID).Update("apply_code", "update.config-drift").Error)
	info := f.check.ServiceUpdates["app"].UpdateInfo
	digest := info.LatestDigest
	info.LatestDigest = "invalid"
	first, err := f.s.CheckPolicyProject(t.Context(), "0", "stack")
	require.NoError(t, err)
	require.Equal(t, "update.check-unavailable", first.PolicyCode)
	info.LatestDigest = digest
	second, err := f.s.CheckPolicyProject(t.Context(), "0", "stack")
	require.NoError(t, err)
	require.Empty(t, second.PolicyCode)
	row, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
	require.NoError(t, err)
	require.Empty(t, row.BookkeepingCode)
	require.NotNil(t, row.BookkeepingAt)
	require.False(t, row.Stale)
	require.Equal(t, "update.config-drift", row.ApplyCode)
	require.Empty(t, f.check.PolicyCode, "the raw producer result remains owned by its caller")
}
