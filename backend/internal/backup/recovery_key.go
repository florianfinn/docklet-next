// Source: getarcaneapp/arcane/backend/internal/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations.

// Package backup owns the shared Rustic backup engine used by volume and
// system backups: typed repository operations, per-repository serialization
// and durable run admission.
package backup

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/backup/keyformat"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
)

var (
	ErrRecoveryKeyNotConfigured = errors.New("recovery key is not configured")
	ErrPolicyValidation         = errors.New("backup.invalid_policy")
	// RecoveryKeyFormat is 8 hyphenated groups of 6 base32 characters, used verbatim as the Rustic password.
	RecoveryKeyFormat = keyformat.RecoveryKeyFormat
)

func ValidateRecoveryKey(key string) error { return keyformat.ValidateRecoveryKey(key) }
func GenerateRecoveryKey() (string, error) { return keyformat.GenerateRecoveryKey() }

// RecoveryKeyStore persists the recovery key shared by the system and volume backup domains.
type RecoveryKeyStore struct {
	db *database.DB
}

func NewRecoveryKeyStore(db *database.DB) *RecoveryKeyStore {
	return &RecoveryKeyStore{db: db}
}

func (s *RecoveryKeyStore) Configured(ctx context.Context) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&SystemBackupRecoveryConfig{}).
		Where("id = ? AND encrypted_recovery_key <> ''", RecoveryKeyConfigID).Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to load recovery key status: %w", err)
	}
	return count > 0, nil
}

// Get returns the decrypted recovery key or ErrRecoveryKeyNotConfigured.
func (s *RecoveryKeyStore) Get(ctx context.Context) (string, error) {
	var config SystemBackupRecoveryConfig
	if err := s.db.WithContext(ctx).
		Where("id = ? AND encrypted_recovery_key <> ''", RecoveryKeyConfigID).
		First(&config).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrRecoveryKeyNotConfigured
		}
		return "", fmt.Errorf("failed to load recovery key: %w", err)
	}
	key, err := crypto.Decrypt(config.EncryptedRecoveryKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt recovery key: %w", err)
	}
	return key, nil
}

func (s *RecoveryKeyStore) Set(ctx context.Context, recoveryKey string) error {
	if err := ValidateRecoveryKey(recoveryKey); err != nil {
		return err
	}
	encrypted, err := crypto.Encrypt(recoveryKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt recovery key: %w", err)
	}
	config := SystemBackupRecoveryConfig{ID: RecoveryKeyConfigID, EncryptedRecoveryKey: encrypted}
	if saveRecoveryKeyErr := s.db.WithContext(ctx).Save(&config).Error; saveRecoveryKeyErr != nil {
		return fmt.Errorf("failed to save recovery key: %w", saveRecoveryKeyErr)
	}
	return nil
}
