// Source: docklet-next/issues/242
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original bounded policy retention and deletion ownership.

package policies

import (
	"context"
	"encoding/json"
	"time"

	dto "github.com/florianfinn/docklet-next/types/updater"
	"gorm.io/gorm"
)

func unsettled(tx *gorm.DB, id string) (bool, error) {
	var count int64
	err := tx.Model(&Child{}).Where("policy_id = ? AND (status NOT IN ? OR code = ?)", id, []string{"settled", "refused"}, "runtime.outcome-unknown").Count(&count).Error
	return count != 0, err
}

func deletePolicy(tx *gorm.DB, id string) error {
	if err := tx.Where("candidate_id IN (?)", tx.Model(&Candidate{}).Select("id").Where("policy_id = ?", id)).Delete(&Notice{}).Error; err != nil {
		return err
	}
	if err := tx.Where("policy_id = ?", id).Delete(&Candidate{}).Error; err != nil {
		return err
	}
	if err := tx.Where("policy_id = ?", id).Delete(&Child{}).Error; err != nil {
		return err
	}
	return tx.Delete(&Policy{}, "id = ?", id).Error
}

// DeleteEnvironment runs inside the environment owner's deletion transaction.
func DeleteEnvironment(tx *gorm.DB, env string) error {
	var rows []Policy
	if err := tx.Where("environment_id = ?", env).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := deletePolicy(tx, row.ID); err != nil {
			return err
		}
	}
	return tx.Where("environment_id = ?", env).Delete(&Notice{}).Error
}

func DeleteStack(tx *gorm.DB, env, id string) error {
	var rows []Policy
	if err := tx.Where("environment_id = ? AND target_kind = ? AND target_id = ?", env, "stack", id).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		held, err := unsettled(tx, row.ID)
		if err != nil {
			return err
		}
		if !held {
			if err := deletePolicy(tx, row.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// PruneAbsent is called only after both complete inventories were validated.
func (s Store) PruneAbsent(ctx context.Context, env, reg string, present map[string]bool, now time.Time) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []Policy
		if err := tx.Where("environment_id = ? AND registration_id = ?", env, reg).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			query := tx.Model(&Policy{}).Where("id = ? AND registration_id = ? AND revision = ?", row.ID, reg, row.Revision)
			if present[row.TargetKind+"\x00"+row.TargetID] {
				if row.MissingSince != nil {
					if err := query.Update("missing_since", nil).Error; err != nil {
						return err
					}
				}
				continue
			}
			if row.MissingSince == nil {
				if err := query.Update("missing_since", now).Error; err != nil {
					return err
				}
				continue
			}
			if row.Mode != "check" || now.Sub(*row.MissingSince) < 7*24*time.Hour {
				continue
			}
			held, err := unsettled(tx, row.ID)
			if err != nil {
				return err
			}
			if !held {
				if err := deletePolicy(tx, row.ID); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s Store) PruneHistory(ctx context.Context, now time.Time) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []Policy
		if err := tx.Find(&rows).Error; err != nil {
			return err
		}
		current := map[string]bool{}
		for _, row := range rows {
			var candidates []dto.UpdateCandidate
			if json.Unmarshal([]byte(row.Candidates), &candidates) != nil {
				return nil
			}
			for _, candidate := range candidates {
				if candidate.EntryID != "" {
					current[candidate.EntryID] = true
				}
			}
		}
		var notices []Notice
		cutoff := now.Add(-30 * 24 * time.Hour)
		if err := tx.Where("created_at < ? AND status IN ?", cutoff, []string{"sent", "failed", "unconfigured"}).Find(&notices).Error; err != nil {
			return err
		}
		for _, notice := range notices {
			if current[notice.ID] {
				continue
			}
			if err := tx.Delete(&Notice{}, "id = ?", notice.ID).Error; err != nil {
				return err
			}
			if err := tx.Delete(&Candidate{}, "id = ?", notice.CandidateID).Error; err != nil {
				return err
			}
		}
		return tx.Where("status IN ? AND settled_at < ? AND code <> ?", []string{"settled", "refused"}, cutoff, "runtime.outcome-unknown").Delete(&Child{}).Error
	})
}
