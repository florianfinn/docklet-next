// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original durable observed-run and failed-candidate recovery storage.

package activity

import (
	"context"
	"encoding/json"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"strings"
	"testing"
	"time"
)

func TestObservedEvidenceGrammar(t *testing.T) {
	for _, mode := range []string{"scheduled", "manual"} {
		for _, tc := range []struct {
			name, status, code, outcome, reason   string
			verified, bound, missing, wantUnknown bool
		}{
			{name: "unbound absent", status: "failed"},
			{name: "unbound recovery", status: "failed", outcome: "restored", wantUnknown: true},
			{name: "success absent", status: "success", verified: true, bound: true},
			{name: "success not-needed", status: "success", outcome: "not-needed", verified: true, bound: true},
			{name: "success unverified", status: "success", bound: true, wantUnknown: true},
			{name: "success restored", status: "success", outcome: "restored", verified: true, bound: true, wantUnknown: true},
			{name: "failed no-effect", status: "failed", outcome: "not-needed", bound: true},
			{name: "failed restored", status: "failed", outcome: "restored", bound: true},
			{name: "failed restore-failed", status: "failed", outcome: "restore-failed", reason: "restore.deploy-failed", bound: true},
			{name: "failed unavailable", status: "failed", outcome: "unavailable", reason: "restore.files-changed", bound: true},
			{name: "failed unavailable invalid reason", status: "failed", outcome: "unavailable", reason: "update.unstable", bound: true, wantUnknown: true},
			{name: "failed missing recovery", status: "failed", bound: true, wantUnknown: true},
			{name: "failed missing evidence", status: "failed", bound: true, missing: true, wantUnknown: true},
			{name: "failed interrupted", status: "failed", code: "runtime.interrupted", outcome: "interrupted", bound: true},
			{name: "failed interrupted absent", status: "failed", code: "runtime.interrupted", bound: true, missing: true},
			{name: "failed interrupted zero-effect", status: "failed", code: "runtime.interrupted", outcome: "not-needed", bound: true},
			{name: "cancelled after effect", status: "cancelled", outcome: "interrupted", bound: true},
			{name: "cancelled no effect", status: "cancelled", outcome: "not-needed", bound: true},
			{name: "cancelled queued", status: "cancelled", bound: true, missing: true},
			{name: "unlisted", status: "failed", outcome: "invented", bound: true, wantUnknown: true},
			{name: "restored verified", status: "failed", outcome: "restored", verified: true, bound: true, wantUnknown: true},
			{name: "restored reason", status: "failed", outcome: "restored", reason: "image.busy", bound: true, wantUnknown: true},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				f := runtimeFixture(t)
				req := f.request()
				row, err := f.admit(req)
				require.NoError(t, err)
				if tc.bound {
					require.NoError(t, f.svc.db.Create(&RuntimeObservedRun{RunID: row.RunID, Kind: "stack", Mode: mode, CreatedAt: f.now}).Error)
				}
				var result *dto.LifecycleResult
				if !tc.missing {
					result = &dto.LifecycleResult{RunID: row.RunID, Verified: tc.verified}
					if tc.status == "failed" {
						result.Code = "update.unstable"
						result.HTTPStatus = 500
					}
					if tc.outcome != "" {
						result.Recovery = &dto.RecoveryResult{Outcome: tc.outcome, Code: tc.reason}
					}
					// Exercise the same real JSON decoder used at ingress, with both copies.
					body, err := json.Marshal(result)
					require.NoError(t, err)
					var decoded dto.LifecycleResult
					require.NoError(t, activityDTO.DecodeRuntimeJSON(body, &decoded))
					item := activityDTO.Activity{ID: row.RunID, ImageLifecycle: &decoded, Metadata: map[string]any{"imageLifecycle": decoded}}
					checkedActivityLifecycle(&item)
					result = item.ImageLifecycle
				}
				got, err := runtimeCheckedTerminalTx(f.svc.db.DB, row, tc.status, tc.code, result)
				require.NoError(t, err)
				require.Equal(t, tc.wantUnknown, got != nil && got.Code == "runtime.outcome-unknown")
			})
		}
	}
}
func TestObservedContradictingCopiesAndBounds(t *testing.T) {
	result := dto.LifecycleResult{RunID: uuid.NewString(), Code: "update.unstable", HTTPStatus: 500, Recovery: &dto.RecoveryResult{Outcome: "restored"}}
	other := result
	other.Verified = true
	item := activityDTO.Activity{ID: result.RunID, ImageLifecycle: &other, Metadata: map[string]any{"imageLifecycle": result}}
	checkedActivityLifecycle(&item)
	require.Equal(t, "runtime.outcome-unknown", item.ImageLifecycle.Code)
	result.RestorePointID = strings.Repeat("x", 37)
	require.Equal(t, "runtime.outcome-unknown", checkedLifecycleResult(result).Code)
}
func TestObservedEvidenceByteBoundary(t *testing.T) {
	for _, extra := range []int{0, 1, 2} {
		t.Run(string(rune('a'+extra)), func(t *testing.T) {
			f := runtimeFixture(t)
			req := f.request()
			row, err := f.admit(req)
			require.NoError(t, err)
			require.NoError(t, f.svc.db.Create(&RuntimeObservedRun{RunID: row.RunID, Kind: "stack", Mode: "scheduled", CreatedAt: f.now}).Error)
			result := dto.LifecycleResult{RunID: row.RunID, Code: "update.unstable", HTTPStatus: 500, Recovery: &dto.RecoveryResult{Outcome: "restored"}}
			body, err := json.Marshal(result)
			require.NoError(t, err)
			for len(body) < dto.MaxRecoveryEvidenceBytes-1200 {
				result.Applied = append(result.Applied, dto.AppliedCandidate{Service: strings.Repeat("s", 128), Repository: strings.Repeat("r", 600), Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("a", 64)})
				body, err = json.Marshal(result)
				require.NoError(t, err)
			}
			padding := dto.MaxRecoveryEvidenceBytes - len(body)
			for i := range result.Applied {
				add := min(1024-len(result.Applied[i].Repository), padding+extra)
				result.Applied[i].Repository += strings.Repeat("x", add)
				padding -= add
				if padding+extra == 0 {
					break
				}
			}
			if extra == 2 {
				result.Applied[0].Repository = result.Applied[0].Repository[:len(result.Applied[0].Repository)-2] + "ä"
			}
			body, err = json.Marshal(result)
			require.NoError(t, err)
			require.Equal(t, dto.MaxRecoveryEvidenceBytes+extra, len(body))
			checked, err := runtimeCheckedTerminalTx(f.svc.db.DB, row, "failed", "runtime.failed", &result)
			require.NoError(t, err)
			require.NoError(t, runtimeWriteObservedTx(f.svc.db.DB, f.now, row, checked), "oversize is normalized before the CHECK")
			require.Equal(t, extra > 0, checked.Code == "runtime.outcome-unknown")
		})
	}
}
func TestObservedCommandValidationAndHash(t *testing.T) {
	f := b1Fixture(t)
	for _, op := range []RuntimeOperation{RuntimeUpdateContainer, RuntimeRestoreContainerImage, RuntimeUpdateProject, RuntimeRestoreProject} {
		t.Run(string(op), func(t *testing.T) {
			c := f.command()
			if op == RuntimeUpdateProject || op == RuntimeRestoreProject {
				c = imageCommand(c, op)
			} else {
				c.Operation = op
				c.Target = RuntimeTarget{Kind: "container", ID: strings.Repeat("a", 64)}
				c.Options.ContainerImage = &dto.ContainerImageRequest{}
			}
			plain, err := runtimeCanonicalizeInternal(c)
			require.NoError(t, err)
			set := func(b *dto.PolicyBinding) {
				if c.Options.Lifecycle != nil {
					c.Options.Lifecycle.Request.AutoRestore = true
					c.Options.Lifecycle.Policy = b
				} else {
					c.Options.ContainerImage.AutoRestore = true
					c.Options.ContainerImage.Policy = b
				}
			}
			set(nil)
			observed, err := runtimeCanonicalizeInternal(c)
			if op == RuntimeRestoreProject || op == RuntimeRestoreContainerImage {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotEqual(t, plain.digest, observed.digest)
			binding := &dto.PolicyBinding{PolicyID: "policy", JobRunID: "window", ChildID: uuid.NewString(), Revision: 1}
			set(binding)
			_, err = runtimeCanonicalizeInternal(c)
			require.Error(t, err, "policy and autoRestore cannot combine")
			if c.Options.Lifecycle != nil {
				c.Options.Lifecycle.Request.AutoRestore = false
			} else {
				c.Options.ContainerImage.AutoRestore = false
			}
			binding.Suppressed = []dto.SuppressedCandidate{{Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("b", 64)}}
			canonical, err := runtimeCanonicalizeInternal(c)
			require.NoError(t, err)
			binding.Suppressed[0].Manifest = "sha256:" + strings.Repeat("c", 64)
			changed, err := runtimeCanonicalizeInternal(c)
			require.NoError(t, err)
			require.NotEqual(t, canonical.digest, changed.digest)
			binding.Suppressed[0].Repository = "bad\nrepository"
			_, err = runtimeCanonicalizeInternal(c)
			require.Error(t, err)
			binding.Suppressed = make([]dto.SuppressedCandidate, 65)
			for i := range binding.Suppressed {
				binding.Suppressed[i] = dto.SuppressedCandidate{Service: "app", Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("a", 64)}
			}
			_, err = runtimeCanonicalizeInternal(c)
			require.Error(t, err)
			binding.Suppressed = binding.Suppressed[:64]
			_, err = runtimeCanonicalizeInternal(c)
			require.NoError(t, err, "exactly 64 valid suppressed tuples are accepted")
		})
	}
}
func TestObservedAdmissionAndReceiptRetention(t *testing.T) {
	f := b1Fixture(t)
	c := imageCommand(f.command(), RuntimeUpdateProject)
	c.Options.Lifecycle.Request.AutoRestore = true
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	var observed RuntimeObservedRun
	require.NoError(t, f.store.svc.db.First(&observed, "run_id = ?", admitted.ActivityID).Error)
	require.Equal(t, "manual", observed.Mode)
	// Same-store acceptance is idempotent and must not create a second marker.
	require.NoError(t, runtimeObserveAdmissionTx(f.store.svc.db.DB, f.store.now, admitted.ActivityID, c))
	runtimeCount(t, f.store.svc.db.DB, &RuntimeObservedRun{}, 1)
	require.NoError(t, f.store.svc.db.Model(&RuntimeReceipt{}).Where("run_id = ?", admitted.ActivityID).Updates(map[string]any{"executor_state": "failed", "executor_result_code": "runtime.failed", "executor_settled_at": f.store.now, "settled_at": f.store.now}).Error)
	child := policy.Child{ID: uuid.NewString(), PolicyID: "policy", JobRunID: "job", Revision: 1, Status: "admitted", RuntimeID: admitted.ActivityID}
	require.NoError(t, f.store.svc.db.Create(&child).Error)
	f.store.now = f.store.now.Add(25 * time.Hour)
	count, err := f.store.svc.pruneRuntimeInternal(t.Context(), 100)
	require.NoError(t, err)
	require.Zero(t, count)
	require.NoError(t, f.store.svc.db.Model(&policy.Child{}).Where("id = ?", child.ID).Update("status", "settled").Error)
	count, err = f.store.svc.pruneRuntimeInternal(t.Context(), 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	runtimeCount(t, f.store.svc.db.DB, &RuntimeObservedRun{}, 0)
}
func TestObservedLocalMissingEvidenceHolds(t *testing.T) {
	f := b1Fixture(t)
	c := imageCommand(f.command(), RuntimeUpdateProject)
	c.Options.Lifecycle.Request.AutoRestore = true
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	id := b1Identity(t, c, admitted.ActivityID)
	require.NoError(t, f.store.tx(func(tx *gorm.DB, now time.Time) error {
		snapshot, _, err := runtimeCompleteTxInternal(tx, now, id, runtimeLocalResultInternal{state: "failed", code: activityDTO.CodeRuntimeFailed, message: "failed", settledAt: now})
		require.NoError(t, err)
		require.Equal(t, activityDTO.RuntimeOutcomeUnknown, snapshot.OutcomeState)
		require.Equal(t, "runtime.outcome-unknown", snapshot.ImageLifecycle.Code)
		return err
	}))
}

func TestObservedZeroEffectCancellationSettlesWithoutHold(t *testing.T) {
	for _, mode := range []string{"scheduled", "manual"} {
		for _, withEvidence := range []bool{false, true} {
			t.Run(mode+"/"+map[bool]string{false: "queued", true: "pre-journal"}[withEvidence], func(t *testing.T) {
				f := runtimeFixture(t)
				req := f.request()
				row, err := f.admit(req)
				require.NoError(t, err)
				require.NoError(t, f.svc.db.Create(&RuntimeObservedRun{RunID: row.RunID, Kind: "stack", Mode: mode, CreatedAt: f.now}).Error)
				result := runtimeLocalResultInternal{state: "cancelled", code: activityDTO.CodeRuntimeCancelled, message: "cancelled", settledAt: f.now}
				if withEvidence {
					result.imageLifecycle = &dto.LifecycleResult{RunID: row.RunID, Recovery: &dto.RecoveryResult{Outcome: "not-needed"}}
				}
				require.NoError(t, f.tx(func(tx *gorm.DB, now time.Time) error {
					snapshot, _, err := runtimeCompleteTxInternal(tx.WithContext(context.WithValue(tx.Statement.Context, gateRunContextKey{}, row.RunID)), now, req.Identity, result)
					require.NoError(t, err)
					require.Equal(t, activityDTO.RuntimeOutcomeKnown, snapshot.OutcomeState)
					require.Equal(t, activityDTO.StatusCancelled, snapshot.Status)
					var receipt RuntimeReceipt
					require.NoError(t, tx.First(&receipt, "run_id = ?", row.RunID).Error)
					require.NotNil(t, receipt.SettledAt)
					require.Nil(t, receipt.UnknownReason)
					var marker RuntimeObservedRun
					require.NoError(t, tx.First(&marker, "run_id = ?", row.RunID).Error)
					require.NotNil(t, marker.SettledAt)
					return err
				}))
			})
		}
	}
}

func TestObservedStoredOutcomeCopyContradiction(t *testing.T) {
	for _, withMetadata := range []bool{false, true} {
		t.Run(map[bool]string{false: "history-gone", true: "metadata-retained"}[withMetadata], func(t *testing.T) {
			f := runtimeFixture(t)
			request := f.request()
			row, err := f.admit(request)
			require.NoError(t, err)
			result := dto.LifecycleResult{RunID: row.RunID, Code: "update.unstable", HTTPStatus: 500, Recovery: &dto.RecoveryResult{Outcome: "restored"}}
			body, err := json.Marshal(result)
			require.NoError(t, err)
			if withMetadata {
				require.NoError(t, runtimeImageResultTx(f.svc.db.DB, row.RunID, &result))
			}
			require.NoError(t, f.svc.db.Create(&RuntimeObservedRun{RunID: row.RunID, Kind: "stack", Mode: "scheduled", Evidence: string(body), Outcome: "unavailable", CreatedAt: f.now, SettledAt: &f.now}).Error)
			require.NoError(t, f.tx(func(tx *gorm.DB, now time.Time) error {
				receipt, err := runtimeEvidenceTxInternal(tx, now, request.Identity, runtimeEvidenceInternal{State: "failed", ResultCode: new("runtime.failed"), SettledAt: &now})
				require.NoError(t, err)
				snapshot, err := runtimeSnapshotTxInternal(tx, receipt)
				require.NoError(t, err)
				require.Equal(t, "runtime.outcome-unknown", snapshot.ImageLifecycle.Code)
				require.Equal(t, activityDTO.RuntimeOutcomeUnknown, snapshot.OutcomeState)
				require.False(t, snapshot.ImageLifecycle.Verified)
				return err
			}))
		})
	}
}
