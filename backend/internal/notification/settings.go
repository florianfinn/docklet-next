// Source: getarcaneapp/arcane/backend/internal/notification/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed APNS mobile push.

package notification

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/validation"
	"github.com/florianfinn/docklet-next/types/notification"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
	"maps"
	"strings"
)

func (s *NotificationService) GetAllSettings(ctx context.Context) ([]NotificationSettings, error) {
	var settings []NotificationSettings
	if err := s.db.WithContext(ctx).Find(&settings).Error; err != nil {
		return nil, fmt.Errorf("failed to get notification settings: %w", err)
	}
	return settings, nil
}

func (s *NotificationService) GetSettingsByProvider(ctx context.Context, provider notifications.NotificationProvider) (*NotificationSettings, error) {
	var setting NotificationSettings
	if err := s.db.WithContext(ctx).Where("provider = ?", provider).First(&setting).Error; err != nil {
		return nil, err
	}
	return &setting, nil
}

func (s *NotificationService) CreateOrUpdateSettings(ctx context.Context, provider notifications.NotificationProvider, enabled bool, localConfig database.JSON) (*NotificationSettings, error) {

	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	var setting NotificationSettings
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("provider = ?", provider).First(&setting).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		existing := setting.Config
		encrypted, err := encryptNotificationConfigCredentialsInternal(provider, localConfig, existing)
		if err != nil {
			return &notification.Problem{Code: "notification.config-invalid", Field: "/config/" + notificationTargetFieldByProviderInternal[provider], Status: 400}
		}
		if _, ok := encrypted["events"]; !ok {
			if events, ok := existing["events"]; ok {
				encrypted["events"] = events
			}
		}
		effective := cloneNotificationConfigInternal(encrypted)
		for _, field := range notificationCredentialFieldsByProviderInternal[provider] {
			if value, ok := effective[field].(string); ok && value != "" {
				plain, err := crypto.Decrypt(value)
				if err != nil {
					return err
				}
				effective[field] = plain
			}
		}
		if err := notifications.ValidateConfig(provider, effective); err != nil {
			return err
		}
		setting.Provider = provider
		setting.Enabled = enabled
		setting.Config = encrypted
		return tx.Save(&setting).Error
	})
	if err != nil {
		return nil, err
	}

	return &setting, nil
}

// RedactNotificationConfigCredentials returns a copy of config with provider credential fields blanked for API responses.
func RedactNotificationConfigCredentials(provider notifications.NotificationProvider, localConfig database.JSON) database.JSON {
	redacted := cloneNotificationConfigInternal(localConfig)
	for _, field := range notificationCredentialFieldsByProviderInternal[provider] {
		value, ok := redacted[field]
		if !ok {
			continue
		}
		if value == "" {
			delete(redacted, field)
			continue
		}
		redacted[field] = ""
	}
	return redacted
}

func encryptNotificationConfigCredentialsInternal(provider notifications.NotificationProvider, localConfig, existingConfig database.JSON) (database.JSON, error) {
	encryptedConfig := cloneNotificationConfigInternal(localConfig)
	preserveConfig := existingConfig
	if provider == notifications.NotificationProviderSignal {
		preserveConfig = signalCredentialPreservationConfigInternal(localConfig, existingConfig)
	}
	if provider == notifications.NotificationProviderEmail {
		preserveConfig = emailCredentialPreservationConfigInternal(localConfig, existingConfig)
	}
	if targetField := notificationTargetFieldByProviderInternal[provider]; targetField != "" {
		currentTarget, _ := existingConfig[targetField].(string)
		nextTarget, _ := encryptedConfig[targetField].(string)
		if strings.TrimSpace(nextTarget) == "" && strings.TrimSpace(currentTarget) != "" {
			nextTarget = currentTarget
			encryptedConfig[targetField] = currentTarget
		}

		storedCredentials := make(map[string]bool, len(notificationCredentialFieldsByProviderInternal[provider]))
		updatedCredentials := make(map[string]bool, len(notificationCredentialFieldsByProviderInternal[provider]))
		for _, field := range notificationCredentialFieldsByProviderInternal[provider] {
			preservedValue, _ := preserveConfig[field].(string)
			updatedValue, _ := encryptedConfig[field].(string)
			storedCredentials[field] = preservedValue != ""
			updatedCredentials[field] = updatedValue != ""
		}

		if err := validation.ValidateCredentialTargetChange(
			targetField,
			currentTarget,
			new(nextTarget),
			func(value string) string {
				return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
			},
			storedCredentials,
			updatedCredentials,
		); err != nil {
			return nil, err
		}
	}
	for _, field := range notificationCredentialFieldsByProviderInternal[provider] {
		value, _ := encryptedConfig[field].(string)
		if value == "" {
			if existingValue, ok := preserveConfig[field].(string); ok && existingValue != "" {
				encryptedConfig[field] = existingValue
			}
			continue
		}

		encrypted, err := encryptNotificationCredentialInternal(value)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt notification credential %q: %w", field, err)
		}
		encryptedConfig[field] = encrypted
	}
	return encryptedConfig, nil
}

func signalCredentialPreservationConfigInternal(localConfig, existingConfig database.JSON) database.JSON {
	preserveConfig := cloneNotificationConfigInternal(existingConfig)
	user, _ := localConfig["user"].(string)
	password, _ := localConfig["password"].(string)
	token, _ := localConfig["token"].(string)

	if strings.TrimSpace(token) != "" {
		delete(preserveConfig, "password")
	}
	if strings.TrimSpace(user) != "" || strings.TrimSpace(password) != "" {
		delete(preserveConfig, "token")
	}

	return preserveConfig
}

func emailCredentialPreservationConfigInternal(localConfig, existingConfig database.JSON) database.JSON {
	preserveConfig := cloneNotificationConfigInternal(existingConfig)
	if authMode, _ := localConfig["authMode"].(string); authMode == string(notifications.EmailAuthModeNone) {
		delete(preserveConfig, "smtpPassword")
	}
	return preserveConfig
}

func encryptNotificationCredentialInternal(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if _, err := crypto.Decrypt(value); err == nil {
		return value, nil
	}
	return crypto.Encrypt(value)
}

func cloneNotificationConfigInternal(localConfig database.JSON) database.JSON {
	if localConfig == nil {
		return database.JSON{}
	}
	cloned := make(database.JSON, len(localConfig))
	maps.Copy(cloned, localConfig)
	return cloned
}

func (s *NotificationService) DeleteSettings(ctx context.Context, provider notifications.NotificationProvider) error {
	if err := s.db.WithContext(ctx).Where("provider = ?", provider).Delete(&NotificationSettings{}).Error; err != nil {
		return fmt.Errorf("failed to delete notification settings: %w", err)
	}
	return nil
}
