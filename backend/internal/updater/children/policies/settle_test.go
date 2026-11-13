// Source: docklet-next/issues/441
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original atomic failed-candidate settlement and retry proofs. M24 recovery evidence, validation and reason contracts.

package policies

import (
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"strings"
	"testing"
	"time"
)

func TestSettleChildFailedCandidates(t *testing.T) {
	for _, outcome := range []string{"restored", "restore-failed", "unavailable", "not-needed", "interrupted", ""} {
		t.Run(outcome, func(t *testing.T) {
			f := newStoreFixture(t)
			p := f.policy(t, "scheduled")
			child, _, err := f.PrepareChild(t.Context(), p, "window")
			require.NoError(t, err)
			require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error { return AdmitChild(tx, &dto.PolicyBinding{ChildID: child.ID}, child.ID) }))
			child.Status = "admitted"
			child.RuntimeID = child.ID
			tuple := dto.AppliedCandidate{Service: "app", Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("a", 64)}
			second := tuple
			second.Service = "worker"
			checked := CheckedChildResult{TargetIdentity: p.TargetIdentity, Code: "update.auto-restored", Applied: []dto.AppliedCandidate{tuple, second, tuple}, SettledAt: time.Now().UTC()}
			if outcome != "" {
				checked.Recovery = &dto.RecoveryResult{Outcome: outcome}
			}
			for range 2 {
				require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error { return SettleChildTx(tx, child, checked) }))
			}
			f.reopen(t)
			rows, err := f.FailedCandidates(t.Context(), p.ID, p.TargetIdentity)
			require.NoError(t, err)
			if outcome == "restored" {
				require.Equal(t, []dto.SuppressedCandidate{tuple, second}, rows)
			} else {
				require.Empty(t, rows)
			}
			other, err := f.FailedCandidates(t.Context(), p.ID, "rebound")
			require.NoError(t, err)
			require.Empty(t, other)
			var got Child
			require.NoError(t, f.db.First(&got, "id = ?", child.ID).Error)
			require.Equal(t, "settled", got.Status)
			// Clearing one target must preserve other policies' records.
			keep := FailedCandidate{PolicyID: "other", TargetIdentity: "other", Repository: tuple.Repository, Platform: tuple.Platform, Manifest: tuple.Manifest, RunID: "other", CreatedAt: checked.SettledAt}
			require.NoError(t, f.db.Create(&keep).Error)
			require.NoError(t, ClearFailedTargetTx(f.db.DB, p.EnvironmentID, p.RegistrationID, p.TargetKind, p.TargetID))
			var count int64
			require.NoError(t, f.db.Model(&FailedCandidate{}).Count(&count).Error)
			require.EqualValues(t, 1, count)
		})
	}
}
func TestSettleChildWriteFailureKeepsAdmission(t *testing.T) {
	f := newStoreFixture(t)
	p := f.policy(t, "scheduled")
	child, _, err := f.PrepareChild(t.Context(), p, "window")
	require.NoError(t, err)
	require.NoError(t, AdmitChild(f.db.DB, &dto.PolicyBinding{ChildID: child.ID}, child.ID))
	child.RuntimeID = child.ID
	require.NoError(t, f.db.Exec("CREATE TRIGGER reject_failed BEFORE INSERT ON image_policy_failed_candidates WHEN NEW.service = 'worker' BEGIN SELECT RAISE(ABORT,'rejected'); END").Error)
	checked := CheckedChildResult{TargetIdentity: p.TargetIdentity, Recovery: &dto.RecoveryResult{Outcome: "restored"}, Applied: []dto.AppliedCandidate{{Service: "app", Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("a", 64)}, {Service: "worker", Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("a", 64)}}, SettledAt: time.Now()}
	require.Error(t, f.db.Transaction(func(tx *gorm.DB) error { return SettleChildTx(tx, child, checked) }))
	var got Child
	require.NoError(t, f.db.First(&got, "id = ?", child.ID).Error)
	require.Equal(t, "admitted", got.Status)
	var count int64
	require.NoError(t, f.db.Model(&FailedCandidate{}).Count(&count).Error)
	require.Zero(t, count, "earlier candidate writes roll back with the failed insert")
	require.NoError(t, f.db.Exec("DROP TRIGGER reject_failed").Error)
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error { return SettleChildTx(tx, child, checked) }))
}

func TestSettleChildUnknownKeepsAdmission(t *testing.T) {
	f := newStoreFixture(t)
	p := f.policy(t, "scheduled")
	child, _, err := f.PrepareChild(t.Context(), p, "window")
	require.NoError(t, err)
	require.NoError(t, AdmitChild(f.db.DB, &dto.PolicyBinding{ChildID: child.ID}, child.ID))
	child.RuntimeID = child.ID
	require.Error(t, f.db.Transaction(func(tx *gorm.DB) error {
		return SettleChildTx(tx, child, CheckedChildResult{Code: "runtime.outcome-unknown", SettledAt: time.Now()})
	}))
	var got Child
	require.NoError(t, f.db.First(&got, "id = ?", child.ID).Error)
	require.Equal(t, "admitted", got.Status)
}
