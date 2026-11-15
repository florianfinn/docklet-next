// Source: docklet-next/issues/441
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original transactional child and failed-manifest settlement.

package policies

import (
	"errors"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// CheckedChildResult comes from checked receipt evidence. TargetIdentity is the
// admitted target's identity, never a later policy revision's replacement identity.
type CheckedChildResult struct {
	TargetIdentity string
	Code           string
	Recovery       *dto.RecoveryResult
	Applied        []dto.AppliedCandidate
	SettledAt      time.Time
}

// The caller owns the transaction. Any failure rolls back child settlement and
// candidate inserts together, leaving the admitted child available to reconcile.
func SettleChildTx(tx *gorm.DB, child Child, checked CheckedChildResult) error {
	var current Child
	if err := tx.First(&current, "id = ?", child.ID).Error; err != nil {
		return err
	}
	if current.Status == "settled" {
		return nil
	}
	if current.Status != "admitted" || current.PolicyID != child.PolicyID || current.RuntimeID != child.RuntimeID {
		return errors.New("child is not admitted")
	}
	if checked.Code == "runtime.outcome-unknown" {
		return errors.New("unknown child outcome")
	}
	if checked.SettledAt.IsZero() {
		return errors.New("missing settlement time")
	}
	if checked.Recovery != nil && checked.Recovery.Outcome == "restored" {
		if checked.TargetIdentity == "" {
			return errors.New("missing admitted target identity")
		}
		for _, tuple := range checked.Applied {
			candidate := FailedCandidate{PolicyID: current.PolicyID, TargetIdentity: checked.TargetIdentity, Service: tuple.Service, Repository: tuple.Repository, Platform: tuple.Platform, Manifest: tuple.Manifest, RunID: current.RuntimeID, CreatedAt: checked.SettledAt}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
				return err
			}
		}
		// A binding carries at most MaxSuppressedCandidates tuples; the oldest records go first.
		keep := tx.Model(&FailedCandidate{}).Select("rowid").Where("policy_id = ? AND target_identity = ?", current.PolicyID, checked.TargetIdentity).
			Order("created_at DESC, service, repository, platform, manifest").Limit(dto.MaxSuppressedCandidates)
		if err := tx.Where("policy_id = ? AND target_identity = ? AND rowid NOT IN (?)", current.PolicyID, checked.TargetIdentity, keep).Delete(&FailedCandidate{}).Error; err != nil {
			return err
		}
	}
	changed := tx.Model(&Child{}).Where("id = ? AND status = ?", current.ID, "admitted").Updates(map[string]any{"status": "settled", "code": checked.Code, "settled_at": checked.SettledAt})
	if changed.Error != nil {
		return changed.Error
	}
	if changed.RowsAffected != 1 {
		return errors.New("child settlement changed")
	}
	return nil
}
