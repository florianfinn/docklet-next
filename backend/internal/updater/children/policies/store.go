// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Atomic policy revisions, runtime admission guard and candidate entry dedup.

package policies

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

func Key(parts ...string) string {
	body, _ := json.Marshal(parts)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

type Store struct{ DB *gorm.DB }

func (s Store) Policy(ctx context.Context, env, reg, target string, kinds ...string) (Policy, error) {
	kind := "stack"
	if len(kinds) > 0 {
		kind = kinds[0]
	}
	var p Policy
	err := s.DB.WithContext(ctx).Where("environment_id = ? AND registration_id = ? AND target_kind = ? AND target_id = ?", env, reg, kind, target).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Policy{ID: Key(env, reg, kind, target), EnvironmentID: env, RegistrationID: reg, TargetKind: kind, TargetID: target, Mode: "check", Candidates: "[]"}, nil
	}
	return p, err
}
func (s Store) SavePolicy(ctx context.Context, p Policy, expected uint64) error {
	if p.TargetKind == "" {
		p.TargetKind = "stack"
	}
	if p.Mode != "check" && p.Mode != "scheduled" {
		return image.Failure("update.invalid-policy", 400)
	}
	if p.Mode == "scheduled" && !p.Window().Valid() || p.Mode == "check" && p.Window() != nil {
		return image.Failure("update.invalid-policy", 400)
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := environmentExists(tx, p.EnvironmentID); err != nil {
			return err
		}
		var old Policy
		err := tx.First(&old, "id = ?", p.ID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if old.Revision != expected {
			return image.Failure("update.policy-changed", 409)
		}
		p.Revision = expected + 1
		if err != nil {
			return tx.Create(&p).Error
		}
		changed := tx.Model(&Policy{}).Where("id = ? AND revision = ?", p.ID, expected).Select("*").Updates(&p)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return image.Failure("update.policy-changed", 409)
		}
		return nil
	})
}
func (s Store) PrepareChild(ctx context.Context, p Policy, job string) (Child, bool, error) {
	var current Policy
	if err := s.DB.WithContext(ctx).First(&current, "id = ? AND registration_id = ? AND revision = ?", p.ID, p.RegistrationID, p.Revision).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Child{}, false, image.Failure("update.policy-changed", 409)
		}
		return Child{}, false, err
	}
	c := Child{ID: uuid.NewString(), PolicyID: p.ID, JobRunID: job, Revision: p.Revision, Status: "prepared"}
	changed := s.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&c)
	if changed.Error != nil {
		return c, false, changed.Error
	}
	if changed.RowsAffected == 0 {
		c = Child{}
		err := s.DB.WithContext(ctx).Where("job_run_id = ? AND policy_id = ?", job, p.ID).Take(&c).Error
		return c, false, err
	}
	return c, true, nil
}

// ValidateAdmission runs inside the same transaction that commits runtime receipt
// and frozen intent. Later policy edits cannot revoke an already accepted child.
func ValidateAdmission(tx *gorm.DB, b *dto.PolicyBinding, env, reg, target, identity, actor string, kinds ...string) error {
	if b == nil {
		return nil
	}
	var p Policy
	kind := "stack"
	if len(kinds) > 0 {
		kind = kinds[0]
	}
	var child Child
	if tx.First(&p, "id = ?", b.PolicyID).Error != nil || tx.First(&child, "id = ?", b.ChildID).Error != nil ||
		p.EnvironmentID != env || p.RegistrationID != reg || p.TargetKind != kind || p.TargetID != target || p.TargetIdentity != identity ||
		p.Mode != "scheduled" || p.Revision != b.Revision || p.ActorID != actor ||
		child.PolicyID != p.ID || child.Revision != p.Revision || child.JobRunID != b.JobRunID || child.Status != "prepared" {
		return image.Failure("update.policy-changed", 409)
	}
	return nil
}
func AdmitChild(tx *gorm.DB, b *dto.PolicyBinding, run string) error {
	if b == nil {
		return nil
	}
	changed := tx.Model(&Child{}).Where("id = ? AND status = ?", b.ChildID, "prepared").Updates(map[string]any{"runtime_id": run, "status": "admitted"})
	if changed.Error != nil {
		return changed.Error
	}
	if changed.RowsAffected != 1 {
		return image.Failure("update.policy-changed", 409)
	}
	return nil
}
func (s Store) RecordCandidate(ctx context.Context, policy Policy, c dto.UpdateCandidate, notice bool) (dto.UpdateCandidate, error) {
	c.NotificationState = "not-requested"
	repository := c.ImageRef
	if named, err := reference.ParseNormalizedNamed(repository); err == nil {
		repository = reference.TrimNamed(named).String()
	}
	id := Key(policy.EnvironmentID, policy.RegistrationID, policy.TargetKind, policy.TargetID, policy.TargetIdentity, repository, c.Platform, c.Version, c.Digest)
	// Preserve already-published stack candidate entries across the v14 identity
	// expansion. New targets always use the kind-and-target-qualified identity.
	if policy.TargetKind == "stack" || policy.TargetKind == "" {
		legacy := Key(policy.EnvironmentID, policy.RegistrationID, policy.TargetIdentity, repository, c.Platform, c.Version, c.Digest)
		var count int64
		if err := s.DB.WithContext(ctx).Model(&Candidate{}).Where("id = ? AND policy_id = ?", legacy, policy.ID).Count(&count).Error; err != nil {
			return c, err
		}
		if count == 1 {
			id = legacy
		}
	}
	if !notice {
		var existing Notice
		err := s.DB.WithContext(ctx).Where("candidate_id = ?", id).Take(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c, nil
		}
		if err != nil {
			return c, err
		}
		c.EntryID, c.NotificationState = existing.ID, existing.Status
		return c, nil
	}
	entry := Notice{ID: uuid.NewString(), CandidateID: id, EnvironmentID: policy.EnvironmentID, Status: "pending", CreatedAt: time.Now().UTC()}
	payload, err := json.Marshal(c)
	if err != nil {
		return c, err
	}
	entry.Payload = string(payload)
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Policy
		if tx.First(&current, "id = ?", policy.ID).Error != nil || current.Revision != policy.Revision || current.RegistrationID != policy.RegistrationID {
			return image.Failure("update.policy-changed", 409)
		}
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Candidate{ID: id, PolicyID: policy.ID, CreatedAt: entry.CreatedAt})
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			entry = Notice{}
			return tx.Where("candidate_id = ?", id).Take(&entry).Error
		}
		return tx.Create(&entry).Error
	})
	c.EntryID = entry.ID
	c.NotificationState = entry.Status
	return c, err
}
