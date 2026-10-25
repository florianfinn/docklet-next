// Source: docklet-next/docs/design/update-policies.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
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

func (s Store) Policy(ctx context.Context, env, reg, target string) (Policy, error) {
	var p Policy
	err := s.DB.WithContext(ctx).Where("environment_id = ? AND registration_id = ? AND target_id = ?", env, reg, target).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Policy{ID: Key(env, reg, target), EnvironmentID: env, RegistrationID: reg, TargetID: target, Mode: "off", Candidates: "[]"}, nil
	}
	return p, err
}
func (s Store) SavePolicy(ctx context.Context, p Policy, expected uint64) error {
	if p.Mode != "off" && p.Mode != "notify" && p.Mode != "apply" {
		return image.Failure("update.invalid-policy", 400)
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
func (s Store) Schedule(ctx context.Context, env, reg string) (HostSchedule, error) {
	var p HostSchedule
	err := s.DB.WithContext(ctx).Where("environment_id = ?", env).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return HostSchedule{ID: Key(env, reg), EnvironmentID: env, RegistrationID: reg, PollingInterval: "0 0 * * * *", AutoUpdateInterval: "0 0 0 * * *", PollingEnabled: true, AutoUpdateEnabled: true}, nil
	}
	if err == nil && p.RegistrationID != reg {
		return HostSchedule{}, image.Failure("update.target-changed", 409)
	}
	return p, err
}
func (s Store) SaveSchedule(ctx context.Context, p HostSchedule, expected uint64) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old HostSchedule
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
		changed := tx.Model(&HostSchedule{}).Where("id = ? AND revision = ?", p.ID, expected).Select("*").Updates(&p)
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
func ValidateAdmission(tx *gorm.DB, b *dto.PolicyBinding, env, reg, target, identity, actor string) error {
	if b == nil {
		return nil
	}
	var p Policy
	var child Child
	if tx.First(&p, "id = ?", b.PolicyID).Error != nil || tx.First(&child, "id = ?", b.ChildID).Error != nil ||
		p.EnvironmentID != env || p.RegistrationID != reg || p.TargetID != target || p.TargetIdentity != identity ||
		p.Mode != "apply" || p.Revision != b.Revision || p.ActorID != actor ||
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
	if !notice {
		return c, nil
	}
	repository := c.ImageRef
	if named, err := reference.ParseNormalizedNamed(repository)
 err == nil {
		repository = reference.TrimNamed(named).String()
	}
	id := Key(policy.EnvironmentID, policy.RegistrationID, policy.TargetIdentity, repository, c.Platform, c.Version, c.Digest)
	entry := Notice{ID: uuid.NewString(), CandidateID: id, EnvironmentID: policy.EnvironmentID, Status: "pending", CreatedAt: time.Now().UTC()}
	payload, err := json.Marshal(c)
	if err != nil {
		return c, err
	}
	entry.Payload = string(payload)
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Policy
		if tx.First(&current, "id = ?", policy.ID).Error != nil || current.Revision != policy.Revision || current.Mode != "notify" {
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
