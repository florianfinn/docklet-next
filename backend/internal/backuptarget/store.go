// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original encrypted target persistence and authority-scoped full-set admission.

package backuptarget

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"time"
	"uuid"

	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store uses the host's encryption context. SessionCurrent must run inside the
// commit transaction; a transport-only check cannot fence a late metadata write.
type Store struct {
	DB             *database.DB
	SessionCurrent func(context.Context, *gorm.DB) error
}

type setting struct {
	Key   string `gorm:"primaryKey"`
	Value string
}

func (setting) TableName() string { return "settings" }
func readSetting(tx *gorm.DB, key string) (string, error) {
	var row setting
	err := tx.First(&row, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return row.Value, err
}
func writeSetting(tx *gorm.DB, key, value string) error {
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&setting{key, value}).Error
}

// ContentHash excludes secrets and fixes the list order. ConfigVersion binds
// credential changes without disclosing credentials to status readers.
func ContentHash(targets []dto.SyncedTarget) (string, error) {
	type public struct {
		dto.TargetLocation
		ID, Name, Username, Options string
		ConfigVersion               int64
	}
	rows := make([]public, 0, len(targets))
	for _, t := range targets {
		rows = append(rows, public{t.TargetLocation, t.ID, t.Name, t.Username, t.Options, t.ConfigVersion})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	bytes, err := json.Marshal(rows)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(bytes)
	return hex.EncodeToString(hash[:]), nil
}

func (s *Store) Apply(ctx context.Context, set dto.TargetSet) (receipt dto.TargetSetReceipt, err error) {
	if set.Authority == "" || set.HubHostID == "" || set.SetVersion < 1 || s.SessionCurrent == nil {
		return receipt, invalid()
	}
	seen := map[string]bool{}
	for _, t := range set.Targets {
		if t.ID == "" || t.ID == "local" || seen[t.ID] || t.ConfigVersion < 1 {
			return receipt, invalid()
		}
		seen[t.ID] = true
		secret := t.Secret
		input, e := Validate(dto.TargetInput{TargetLocation: t.TargetLocation, Name: t.Name, Username: t.Username, Secret: &secret, Options: t.Options, RecoveryKey: t.RecoveryKey})
		if e != nil {
			return receipt, e
		}
		if !sameLocation(input.TargetLocation, t.TargetLocation) || input.Name != t.Name || input.Options != t.Options || input.RecoveryKey == "" {
			return receipt, invalid()
		}
		if t.HostID != nil && *t.HostID != set.HubHostID {
			return receipt, dto.NewProblem(422, "backup.target.wrong-host", nil)
		}
	}
	hash, err := ContentHash(set.Targets)
	if err != nil {
		return receipt, err
	}
	if hash != set.ContentHash {
		return receipt, invalid()
	}
	receipt = dto.TargetSetReceipt{Authority: set.Authority, SetVersion: set.SetVersion, ContentHash: hash, Conflicts: []string{}}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		authority, e := readSetting(tx, "backupTargetAuthority")
		if e != nil {
			return e
		}
		host, e := readSetting(tx, "hubHostId")
		if e != nil {
			return e
		}
		versionText, e := readSetting(tx, "backupTargetSetVersion")
		if e != nil {
			return e
		}
		version, e := strconv.ParseInt(defaultString(versionText, "0"), 10, 64)
		if e != nil {
			return e
		}
		previousHash, e := readSetting(tx, "backupTargetContentHash")
		if e != nil {
			return e
		}
		if authority == set.Authority && host == set.HubHostID {
			if set.SetVersion < version || set.SetVersion == version && hash != previousHash {
				return dto.NewProblem(409, "backup.target.stale-set", nil)
			}
			if set.SetVersion == version {
				for _, target := range set.Targets {
					var held Target
					if e := tx.First(&held, "id = ?", target.ID).Error; e != nil {
						return e
					}
					if !sameLocation(held.location(), target.TargetLocation) {
						receipt.Conflicts = append(receipt.Conflicts, target.ID)
					}
				}
				return s.SessionCurrent(ctx, tx)
			}
		}
		var held []Target
		if e = tx.Find(&held).Error; e != nil {
			return e
		}
		byID := map[string]Target{}
		for _, row := range held {
			byID[row.ID] = row
		}
		now := time.Now().UTC()
		// Retire first so active names may be reused by a new immutable ID.
		if e = tx.Model(&Target{}).Where("retired_at IS NULL").Update("retired_at", now).Error; e != nil {
			return e
		}
		for _, t := range set.Targets {
			old, exists := byID[t.ID]
			if exists && !sameLocation(old.location(), t.TargetLocation) {
				receipt.Conflicts = append(receipt.Conflicts, t.ID)
				if e = tx.Model(&Target{}).Where("id = ?", old.ID).Update("retired_at", old.RetiredAt).Error; e != nil {
					return e
				}
				continue
			}
			secret, e := encryptOptional(t.Secret)
			if e != nil {
				return e
			}
			key, e := crypto.Encrypt(t.RecoveryKey)
			if e != nil {
				return e
			}
			row := Target{ID: t.ID, Name: t.Name, Kind: t.Kind, HostID: t.HostID, Path: t.Path, Server: t.Server, Share: t.Share, Username: t.Username, SecretEnc: secret, Options: t.Options, RecoveryKeyEnc: key, ConfigVersion: t.ConfigVersion, CreatedAt: now}
			if exists {
				row.CreatedAt = old.CreatedAt
				row.UpdatedAt = &now
			}
			if e = tx.Save(&row).Error; e != nil {
				return e
			}
		}
		for key, value := range map[string]string{"backupTargetAuthority": set.Authority, "hubHostId": set.HubHostID, "backupTargetSetVersion": strconv.FormatInt(set.SetVersion, 10), "backupTargetContentHash": hash} {
			if e = writeSetting(tx, key, value); e != nil {
				return e
			}
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		return s.SessionCurrent(ctx, tx)
	})
	return receipt, err
}
func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// Authority is read or initialized transactionally by the hub, never from a clock.
func (s *Store) Authority(ctx context.Context) (authority string, version int64, err error) {
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var e error
		authority, e = readSetting(tx, "backup_target_authority")
		if e != nil {
			return e
		}
		text, e := readSetting(tx, "backup_target_set_version")
		if e != nil {
			return e
		}
		if authority == "" {
			authority = uuid.New().String()
			version = 1
			if e = writeSetting(tx, "backup_target_authority", authority); e != nil {
				return e
			}
			return writeSetting(tx, "backup_target_set_version", "1")
		}
		version, e = strconv.ParseInt(text, 10, 64)
		return e
	})
	return
}

// ResetAuthority invalidates every pre-restore set receipt before runtime activation.
func (s *Store) ResetAuthority(ctx context.Context) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := writeSetting(tx, "backup_target_authority", uuid.New().String()); err != nil {
			return err
		}
		return nil
	})
}

// Resolve preserves retired definitions for historical snapshots; capture callers
// request activeOnly so retirement cannot admit a new reference.
func (s *Store) Resolve(ctx context.Context, id, hostID string, activeOnly bool) (dto.SyncedTarget, error) {
	var row Target
	if err := s.DB.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.SyncedTarget{}, dto.NewProblem(422, "backup.target.unknown", nil)
		}
		return dto.SyncedTarget{}, err
	}
	if activeOnly && row.RetiredAt != nil {
		return dto.SyncedTarget{}, dto.NewProblem(422, "backup.target.retired", nil)
	}
	if row.HostID != nil && *row.HostID != hostID {
		return dto.SyncedTarget{}, dto.NewProblem(422, "backup.target.wrong-host", nil)
	}
	key, err := crypto.Decrypt(row.RecoveryKeyEnc)
	if err != nil {
		return dto.SyncedTarget{}, err
	}
	if backup.ValidateRecoveryKey(key) != nil {
		return dto.SyncedTarget{}, invalid()
	}
	secret, err := decryptOptional(row.SecretEnc)
	if err != nil {
		return dto.SyncedTarget{}, err
	}
	return dto.SyncedTarget{TargetLocation: row.location(), ID: row.ID, Name: row.Name, Username: row.Username, Secret: secret, Options: row.Options, RecoveryKey: key, ConfigVersion: row.ConfigVersion}, nil
}

func encryptOptional(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	return crypto.Encrypt(value)
}
func decryptOptional(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	return crypto.Decrypt(value)
}

// SetForHost snapshots authority, version and all usable target definitions in
// one read transaction. Secrets enter only the authenticated sync envelope.
func (s *Store) SetForHost(ctx context.Context, hostID string) (set dto.TargetSet, err error) {
	if _, _, err = s.Authority(ctx); err != nil {
		return set, err
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var e error
		set.Authority, e = readSetting(tx, "backup_target_authority")
		if e != nil {
			return e
		}
		text, e := readSetting(tx, "backup_target_set_version")
		if e != nil {
			return e
		}
		set.SetVersion, e = strconv.ParseInt(text, 10, 64)
		if e != nil {
			return e
		}
		set.HubHostID = hostID
		var rows []Target
		if e = tx.Where("retired_at IS NULL AND (host_id IS NULL OR host_id = ?)", hostID).Order("id").Find(&rows).Error; e != nil {
			return e
		}
		set.Targets = []dto.SyncedTarget{}
		for _, row := range rows {
			secret, e := decryptOptional(row.SecretEnc)
			if e != nil {
				return e
			}
			key, e := crypto.Decrypt(row.RecoveryKeyEnc)
			if e != nil {
				return e
			}
			set.Targets = append(set.Targets, dto.SyncedTarget{TargetLocation: row.location(), ID: row.ID, Name: row.Name, Username: row.Username, Secret: secret, Options: row.Options, RecoveryKey: key, ConfigVersion: row.ConfigVersion})
		}
		set.ContentHash, e = ContentHash(set.Targets)
		return e
	})
	return
}
