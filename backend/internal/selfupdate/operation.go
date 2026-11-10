// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	dto "github.com/florianfinn/docklet-next/types/system"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const OperationKey = "selfupdate.operation"
const OperationTimeout = 10 * time.Minute

type Operation struct {
	OpID               string    `json:"opId"`
	Role               string    `json:"role"`
	RunID              string    `json:"runId,omitempty"`
	EnvironmentID      string    `json:"environmentId"`
	CredentialRevision int64     `json:"credentialRevision,omitempty"`
	OldContainerID     string    `json:"oldContainerId"`
	TargetRef          string    `json:"targetRef"`
	TargetVersion      string    `json:"targetVersion"`
	FromVersion        string    `json:"fromVersion"`
	Phase              string    `json:"phase"`
	Outcome            string    `json:"outcome,omitempty"`
	Code               string    `json:"code,omitempty"`
	Deadline           time.Time `json:"deadline"`
}

func (o Operation) DTO(running bool) dto.SelfUpdateOperation {
	return dto.SelfUpdateOperation{OpID: o.OpID, Phase: o.Phase, Outcome: o.Outcome, Code: o.Code, TargetVersion: o.TargetVersion, RunID: o.RunID, HelperRunning: running}
}

type Store struct {
	DB  *database.DB
	Key string
}

func NewStore(db *database.DB) *Store { return &Store{DB: db, Key: OperationKey} }
func (s *Store) Read(ctx context.Context) (*Operation, error) {
	raw, ok, err := kv.NewKVService(s.DB).Get(ctx, s.Key)
	if err != nil || !ok {
		return nil, err
	}
	var op Operation
	if err := json.Unmarshal([]byte(raw), &op); err != nil {
		return nil, err
	}
	return &op, nil
}

// Reserve persists operation and optional run together, before any remote effect.
// A repeated operation ID is read-only; a different unresolved ID is refused.
func (s *Store) Reserve(ctx context.Context, op Operation, reserve func(*gorm.DB, *Operation) error) (Operation, bool, error) {
	var result Operation
	created := false
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var entry kv.KVEntry
		err := tx.First(&entry, "key=?", s.Key).Error
		if err == nil {
			if err := json.Unmarshal([]byte(entry.Value), &result); err != nil {
				return err
			}
			if result.OpID == op.OpID {
				if result.TargetRef != op.TargetRef || result.TargetVersion != op.TargetVersion {
					return Refuse("update.invalid-target")
				}
				return nil
			}
			if result.Phase != "settled" {
				return Refuse("host.busy")
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !ValidOpID(op.OpID) || op.TargetRef == "" || op.Deadline.IsZero() {
			return Refuse("update.invalid-target")
		}
		op.Phase = "reserved"
		if reserve != nil {
			if err := reserve(tx, &op); err != nil {
				return err
			}
		}
		raw, err := json.Marshal(op)
		if err != nil {
			return err
		}
		entry = kv.KVEntry{Key: s.Key, Value: string(raw)}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"})}).Create(&entry).Error; err != nil {
			return err
		}
		result = op
		created = true
		return nil
	})
	return result, created, err
}

// Change uses an exact-value CAS and a transaction so terminal run evidence and
// operation settlement cannot diverge on a database write failure or restart.
func (s *Store) Change(ctx context.Context, id string, change func(*gorm.DB, *Operation) error) (bool, error) {
	changed := false
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var entry kv.KVEntry
		if err := tx.First(&entry, "key=?", s.Key).Error; err != nil {
			return err
		}
		var op Operation
		if err := json.Unmarshal([]byte(entry.Value), &op); err != nil {
			return err
		}
		if op.OpID != id || op.Phase == "settled" {
			return nil
		}
		if err := change(tx, &op); err != nil {
			return err
		}
		raw, err := json.Marshal(op)
		if err != nil {
			return err
		}
		update := tx.Model(&kv.KVEntry{}).Where("key=? AND value=?", s.Key, entry.Value).Updates(map[string]any{"value": string(raw), "updated_at": time.Now()})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errors.New("self-update operation changed during settlement")
		}
		changed = true
		return nil
	})
	return changed, err
}
