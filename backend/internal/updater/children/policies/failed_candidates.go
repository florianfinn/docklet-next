// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original durable observed-run and failed-candidate recovery storage.

package policies

import (
	"context"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"gorm.io/gorm"
	"time"
)

type FailedCandidate struct {
	PolicyID       string    `gorm:"primaryKey;not null"`
	TargetIdentity string    `gorm:"primaryKey;not null"`
	Service        string    `gorm:"primaryKey;not null"`
	Repository     string    `gorm:"primaryKey;not null"`
	Platform       string    `gorm:"primaryKey;not null"`
	Manifest       string    `gorm:"primaryKey;not null"`
	RunID          string    `gorm:"not null"`
	CreatedAt      time.Time `gorm:"not null"`
}

func (FailedCandidate) TableName() string { return "image_policy_failed_candidates" }

func (s Store) FailedCandidates(ctx context.Context, policy, identity string) ([]dto.SuppressedCandidate, error) {
	var rows []FailedCandidate
	err := s.DB.WithContext(ctx).Where("policy_id = ? AND target_identity = ?", policy, identity).
		Order("service, repository, platform, manifest").Find(&rows).Error
	result := make([]dto.SuppressedCandidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, dto.SuppressedCandidate{Service: row.Service, Repository: row.Repository, Platform: row.Platform, Manifest: row.Manifest})
	}
	return result, err
}

// ClearFailedTargetTx clears all recorded identities for policies of the exact target.
func ClearFailedTargetTx(tx *gorm.DB, environment, registration, kind, target string) error {
	policies := tx.Model(&Policy{}).Select("id").Where("environment_id = ? AND registration_id = ? AND target_kind = ? AND target_id = ?", environment, registration, kind, target)
	return tx.Where("policy_id IN (?)", policies).Delete(&FailedCandidate{}).Error
}
