// Source: getarcaneapp/arcane/backend/internal/settings/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed APNS mobile push, template catalogue, GitOps, Swarm, standalone build workshop, OIDC and passkeys.

package settings

import (
	"context"
	"crypto/mldsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"uuid"

	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
)

func (s *SettingsService) EnsureEncryptionKey(ctx context.Context) (string, error) {
	s.writes.Lock()
	defer s.writes.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}

	const keyName = "encryptionKey"
	var key string

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sv SettingVariable
		err := tx.Where("key = ?", keyName).First(&sv).Error

		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("failed to load encryption key: %w", err)
		}

		if sv.Value != "" {
			key = sv.Value
			return nil
		}

		notFound := errors.Is(err, gorm.ErrRecordNotFound)
		sum := sha256.Sum256([]byte(uuid.New().String()))
		generatedKey := base64.StdEncoding.EncodeToString(sum[:])
		key = generatedKey

		if notFound {
			if createErr := tx.Create(&SettingVariable{Key: keyName, Value: generatedKey}).Error; createErr != nil {
				return fmt.Errorf("failed to persist encryption key: %w", createErr)
			}
			return nil
		}

		if updErr := tx.Model(&SettingVariable{}).
			Where("key = ?", keyName).
			Update("value", generatedKey).Error; updErr != nil {
			return fmt.Errorf("failed to update encryption key: %w", updErr)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	return key, nil
}

func (s *SettingsService) EnsureJwtSigningKey(ctx context.Context) (*mldsa.PrivateKey, error) {
	seed, err := s.ensureEncryptedKeyInternal(ctx, "jwtSigningKeySeed", mldsa.PrivateKeySize)
	if err != nil {
		return nil, err
	}
	key, err := mldsa.NewPrivateKey(mldsa.MLDSA87(), seed)
	if err != nil {
		return key, fmt.Errorf("failed to load jwt signing key: %w", err)
	}
	return key, nil
}

func (s *SettingsService) EnsureBrowserSessionSigningKey(ctx context.Context) ([]byte, error) {
	return s.ensureEncryptedKeyInternal(ctx, "browserSessionSigningKey", browserSessionSigningKeySize)
}

func (s *SettingsService) ensureEncryptedKeyInternal(ctx context.Context, keyName string, size int) ([]byte, error) {
	s.writes.Lock()
	defer s.writes.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var key []byte

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sv SettingVariable
		err := tx.Where("key = ?", keyName).First(&sv).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("failed to load signing key: %w", err)
		}

		if sv.Value != "" {
			decoded, decErr := crypto.Decrypt(sv.Value)
			if decErr != nil {
				return fmt.Errorf("failed to decrypt signing key: %w", decErr)
			}
			key, decErr = base64.StdEncoding.DecodeString(decoded)
			if decErr != nil {
				return fmt.Errorf("failed to decode signing key: %w", decErr)
			}
			if len(key) != size {
				return fmt.Errorf("invalid signing key length: got %d, want %d", len(key), size)
			}
			return nil
		}

		key = make([]byte, size)
		if _, genErr := rand.Read(key); genErr != nil {
			return fmt.Errorf("failed to generate signing key: %w", genErr)
		}
		encrypted, encErr := crypto.Encrypt(base64.StdEncoding.EncodeToString(key))
		if encErr != nil {
			return fmt.Errorf("failed to encrypt signing key: %w", encErr)
		}

		if errors.Is(err, gorm.ErrRecordNotFound) {
			if createSigningKeyErr := tx.Create(&SettingVariable{Key: keyName, Value: encrypted}).Error; createSigningKeyErr != nil {
				return fmt.Errorf("failed to persist signing key: %w", createSigningKeyErr)
			}
			return nil
		}
		if updateSigningKeyErr := tx.Model(&SettingVariable{}).
			Where("key = ?", keyName).
			Update("value", encrypted).Error; updateSigningKeyErr != nil {
			return fmt.Errorf("failed to update signing key: %w", updateSigningKeyErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return key, nil
}
