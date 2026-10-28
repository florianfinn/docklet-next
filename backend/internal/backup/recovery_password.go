// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original verified local repository password attachment without re-keying.

package backup

import (
	"context"
	"errors"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm/clause"
)

// SetVerifiedPassword accepts the password already checked against a repository.
// Derived local passwords predate the grouped recovery-key grammar.
func (s *RecoveryKeyStore) SetVerifiedPassword(ctx context.Context, password string) error {
	if password == "" {
		return errors.New("empty repository password")
	}
	encrypted, err := crypto.Encrypt(password)
	if err != nil {
		return err
	}
	row := SystemBackupRecoveryConfig{EncryptedRecoveryKey: encrypted}
	row.ID = RecoveryKeyConfigID
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"encrypted_recovery_key"})}).Create(&row).Error
}
