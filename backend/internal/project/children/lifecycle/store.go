// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Executor-local encrypted deployment evidence and durable pin ownership.

package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Baseline struct {
	TargetID       string `gorm:"primaryKey;not null"`
	TargetIdentity string `gorm:"not null"`
	RegistrationID string `gorm:"not null"`
	SourceRunID    string `gorm:"not null"`
	Revision       string `gorm:"not null"`
	Payload        string `gorm:"not null"`
	Available      bool   `gorm:"not null"`
}

func (Baseline) TableName() string { return "deployment_baselines" }

type Point struct {
	ID             string    `gorm:"primaryKey;not null"`
	TargetID       string    `gorm:"not null;index:idx_restore_points_target_created,priority:1"`
	TargetIdentity string    `gorm:"not null"`
	RegistrationID string    `gorm:"not null"`
	SourceRunID    string    `gorm:"not null"`
	CreatedAt      time.Time `gorm:"not null;index:idx_restore_points_target_created,priority:2"`
	LastRestoredAt *time.Time
	FailedAttempt  bool   `gorm:"not null;default:false"`
	SourceState    string `gorm:"not null"`
	Revision       string `gorm:"not null"`
	Payload        string `gorm:"not null"`
	Available      bool   `gorm:"not null"`
}

func (Point) TableName() string { return "restore_points" }

// Run holds outlive worker/lease epochs; only a settled runtime receipt releases them.
type Run struct {
	ID              string `gorm:"primaryKey;not null"`
	PointID         string `gorm:"not null"`
	SelectedPointID string `gorm:"not null"`
	HadEffect       bool   `gorm:"not null"`
	Settled         bool   `gorm:"not null"`
}

func (Run) TableName() string { return "image_lifecycle_runs" }

type File struct {
	Path    string
	Content []byte
	Present bool
}
type ServiceImage struct {
	Reference, ImageID, Digest, Platform, ContainerID, ContainerName, Service, ConfigHash string
	Present, Running, OneOff                                                              bool
}
type Snapshot struct {
	AutoExcluded                                                   bool `json:"-"`
	TargetID, TargetIdentity, RegistrationID, RunID, SavedRevision string
	Files                                                          []File
	Model                                                          *compose.Project
	Services                                                       map[string]ServiceImage
	Containers                                                     map[string]ServiceImage
}
type Store struct{ DB *database.DB }

func Hash(value any) string {
	body, _ := json.Marshal(value)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func Pin(kind, owner, service string) string {
	return guard.ReservedHost + "/" + kind + "/" + Hash([]string{kind, owner}) + "/" + Hash([]string{"service", service}) + map[string]string{"points": ":saved", "runs": ":frozen"}[kind]
}
func seal(snapshot *Snapshot) (string, error) {
	if !fitsRestoreIntent(snapshot) {
		return "", imageDTO.Failure("update.snapshot-unavailable", 409)
	}
	if settings.ValidateCredentialEncryption() != nil {
		return "", imageDTO.Failure("update.snapshot-unavailable", 409)
	}
	body, err := json.Marshal(snapshot)
	if err != nil || len(body) > 8<<20 {
		return "", imageDTO.Failure("update.snapshot-unavailable", 409)
	}
	encrypted, err := crypto.Encrypt(string(body))
	if err != nil {
		return "", imageDTO.Failure("update.snapshot-unavailable", 409)
	}
	return encrypted, nil
}
func Open(payload string) (*Snapshot, error) {
	if settings.ValidateCredentialEncryption() != nil {
		return nil, imageDTO.Failure("update.snapshot-unavailable", 409)
	}
	body, err := crypto.Decrypt(payload)
	if err != nil {
		return nil, imageDTO.Failure("restore.unavailable", 409)
	}
	var out Snapshot
	if json.Unmarshal([]byte(body), &out) != nil || out.Model == nil || len(out.Files) == 0 {
		return nil, imageDTO.Failure("restore.unavailable", 409)
	}
	return &out, nil
}
func (s Store) SaveBaseline(ctx context.Context, snapshot *Snapshot) error {
	payload, err := seal(snapshot)
	if err != nil {
		return err
	}
	row := Baseline{TargetID: snapshot.TargetID, TargetIdentity: snapshot.TargetIdentity, RegistrationID: snapshot.RegistrationID, SourceRunID: snapshot.RunID, Revision: Hash(snapshot), Payload: payload, Available: true}
	return s.DB.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}
func (s Store) Invalidate(ctx context.Context, id string) error {
	return s.DB.WithContext(ctx).Model(&Baseline{}).Where("target_id = ?", id).Update("available", false).Error
}
func (s Store) Baseline(ctx context.Context, current *Snapshot) (*Snapshot, string, error) {
	var row Baseline
	err := s.DB.WithContext(ctx).First(&row, "target_id = ?", current.TargetID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && (!row.Available || row.TargetIdentity != current.TargetIdentity || row.RegistrationID != current.RegistrationID) {
		return nil, "", imageDTO.Failure("update.baseline-unavailable", 409)
	}
	if err != nil {
		return nil, "", imageDTO.Failure("update.persistence-failed", 500)
	}
	if settings.ValidateCredentialEncryption() != nil {
		return nil, "", imageDTO.Failure("update.snapshot-unavailable", 409)
	}
	saved, err := Open(row.Payload)
	if err != nil {
		return nil, "", imageDTO.Failure("update.baseline-unavailable", 409)
	}
	return saved, row.Revision, nil
}
func (s Store) Point(ctx context.Context, id string) (Point, *Snapshot, error) {
	var row Point
	if err := s.DB.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return row, nil, imageDTO.Failure("restore.not-found", 404)
		}
		return row, nil, imageDTO.Failure("update.persistence-failed", 500)
	}
	if !row.Available {
		return row, nil, imageDTO.Failure("restore.unavailable", 409)
	}
	snap, err := Open(row.Payload)
	return row, snap, err
}

// Prepare commits protection before installing tags. Availability is published
// separately only after every retention tag was verified under the host lease.
func (s Store) Prepare(ctx context.Context, snapshot *Snapshot, run, selected, state string) (Point, error) {
	payload, err := seal(snapshot)
	if err != nil {
		return Point{}, err
	}
	point := Point{ID: uuid.NewString(), TargetID: snapshot.TargetID, TargetIdentity: snapshot.TargetIdentity, RegistrationID: snapshot.RegistrationID, SourceRunID: run, CreatedAt: time.Now().UTC(), SourceState: state, Revision: Hash(snapshot), Payload: payload}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if selected != "" {
			var recent []Point
			if err := tx.Where("target_id = ? AND last_restored_at IS NOT NULL", snapshot.TargetID).Order("last_restored_at DESC").Limit(1).Find(&recent).Error; err != nil {
				return err
			}
			used := point.CreatedAt
			if len(recent) > 0 && !used.After(*recent[0].LastRestoredAt) {
				used = recent[0].LastRestoredAt.Add(time.Microsecond)
			}
			updated := tx.Model(&Point{}).Where("id = ? AND target_id = ? AND target_identity = ? AND registration_id = ? AND available = ?", selected, snapshot.TargetID, snapshot.TargetIdentity, snapshot.RegistrationID, true).Update("last_restored_at", used)
			if updated.Error != nil || updated.RowsAffected != 1 {
				return imageDTO.Failure("restore.unavailable", 409)
			}
		}
		if err := tx.Create(&point).Error; err != nil {
			return err
		}
		for _, img := range snapshot.Images() {
			if !img.Present {
				continue
			}
			pin := guard.Protection{ImageID: img.ImageID, Reason: "restore-point", OwnerID: point.ID}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&pin).Error; err != nil {
				return err
			}
		}
		return tx.Create(&Run{ID: run, PointID: point.ID, SelectedPointID: selected}).Error
	})
	if err != nil {
		return point, imageDTO.Failure("update.persistence-failed", 500)
	}
	return point, nil
}
func (s Store) Available(ctx context.Context, id string) error {
	r := s.DB.WithContext(ctx).Model(&Point{}).Where("id = ?", id).Update("available", true)
	if r.Error != nil || r.RowsAffected != 1 {
		return imageDTO.Failure("update.persistence-failed", 500)
	}
	return nil
}

// A healthy pre-attempt snapshot remains baseline evidence even when its run
// fails. Persist failed-state classification before the receipt can settle.
func (s Store) FailedAttempt(ctx context.Context, id string) error {
	return s.DB.WithContext(ctx).Model(&Point{}).Where("id = ? AND source_state = ?", id, "partial-source").Update("failed_attempt", true).Error
}
func (s Store) Effect(ctx context.Context, run string) error {
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record Run
		if err := tx.First(&record, "id = ?", run).Error; err != nil {
			return imageDTO.Failure("update.persistence-failed", 500)
		}
		var point Point
		if err := tx.First(&point, "id = ?", record.PointID).Error; err != nil {
			return imageDTO.Failure("update.persistence-failed", 500)
		}
		if err := tx.Model(&Run{}).Where("id = ?", run).Update("had_effect", true).Error; err != nil {
			return err
		}
		return tx.Model(&Baseline{}).Where("target_id = ?", point.TargetID).Update("available", false).Error
	})
	if err != nil {
		return imageDTO.Failure("update.persistence-failed", 500)
	}
	return nil
}

func (s Store) Held(ctx context.Context, id string) (bool, error) {
	var n int64
	if err := s.DB.WithContext(ctx).Model(&Run{}).Where("(point_id = ? OR selected_point_id = ?) AND settled = ?", id, id, false).Count(&n).Error; err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	err := s.DB.WithContext(ctx).Table("runtime_image_intents AS i").Joins("JOIN runtime_receipts r ON r.run_id = i.run_id").Where("r.kind = 'receipt' AND r.settled_at IS NULL AND json_extract(i.payload, '$.request.pointId') = ?", id).Count(&n).Error
	return n > 0, err
}

func (p Point) DTO(orphaned bool) dto.RestorePoint {
	return dto.RestorePoint{ID: p.ID, RegistrationID: p.RegistrationID, TargetID: p.TargetID, TargetIdentity: p.TargetIdentity, SourceRunID: p.SourceRunID, CreatedAt: p.CreatedAt, SourceState: p.SourceState, Available: p.Available, Orphaned: orphaned}
}

// Delete serializes the active-run hold check and metadata deletion with runtime admission.
func (s Store) Delete(ctx context.Context, id string) error {
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		held, err := (Store{DB: &database.DB{DB: tx}}).Held(ctx, id)
		if err != nil {
			return imageDTO.Failure("update.persistence-failed", 500)
		}
		if held {
			return imageDTO.Failure("restore.in-use", 409)
		}
		result := tx.Delete(&Point{}, "id = ?", id)
		if result.Error != nil {
			return imageDTO.Failure("update.persistence-failed", 500)
		}
		if result.RowsAffected != 1 {
			return imageDTO.Failure("restore.not-found", 404)
		}
		return nil
	})
	return err
}
