// Source: docklet-next/issues/680
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age implementation.

package policies

import (
	"context"
	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/identity"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type FirstSeen struct {
	Repository  string    `gorm:"primaryKey;not null"`
	Platform    string    `gorm:"primaryKey;not null"`
	Manifest    string    `gorm:"primaryKey;not null"`
	FirstSeenAt time.Time `gorm:"not null"`
	LastSeenAt  time.Time `gorm:"not null;index"`
}

func (FirstSeen) TableName() string { return "image_update_first_seen" }
func ageKey(ref, platform, manifest string) (FirstSeen, error) {
	n, e := reference.ParseNormalizedNamed(ref)
	if e != nil {
		return FirstSeen{}, e
	}
	p, e := identity.ParsePlatform(platform)
	if e != nil {
		return FirstSeen{}, e
	}
	return FirstSeen{Repository: reference.TrimNamed(n).Name(), Platform: p.String(), Manifest: manifest}, nil
}

// ObserveFirstSeen commits the first observation before any scheduled admission.
func (s Store) ObserveFirstSeen(ctx context.Context, c dto.UpdateCandidate, now time.Time) error {
	if !dto.ValidManifest(c.Manifest) {
		return nil
	}
	row, e := ageKey(c.ImageRef, c.Platform, c.Manifest)
	if e != nil {
		return e
	}
	row.FirstSeenAt, row.LastSeenAt = now, now
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Expire before touching the observed key so returning candidates wait again.
		if err := tx.Where("last_seen_at < ?", now.Add(-120*24*time.Hour)).Delete(&FirstSeen{}).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "repository"}, {Name: "platform"}, {Name: "manifest"}}, DoUpdates: clause.AssignmentColumns([]string{"last_seen_at"})}).Create(&row).Error
	})
}

// AgeCandidates owns its returned slice and never adds dates to cached candidates.
func (s Store) AgeCandidates(ctx context.Context, source []dto.UpdateCandidate, days int, now time.Time) ([]dto.UpdateCandidate, error) {
	out := make([]dto.UpdateCandidate, len(source))
	copy(out, source)
	for i := range out {
		out[i].EligibleAt = nil
		if days == 0 || !dto.ValidManifest(out[i].Manifest) {
			continue
		}
		key, e := ageKey(out[i].ImageRef, out[i].Platform, out[i].Manifest)
		if e != nil {
			return nil, e
		}
		var row FirstSeen
		found := s.DB.WithContext(ctx).Where(&key).Limit(1).Find(&row)
		if found.Error != nil {
			return nil, found.Error
		}
		if found.RowsAffected == 0 {
			out[i].Manifest = ""
			continue
		}
		at := row.FirstSeenAt.Add(time.Duration(days) * 24 * time.Hour)
		if at.After(now) {
			out[i].EligibleAt = &at
		}
	}
	return out, nil
}
