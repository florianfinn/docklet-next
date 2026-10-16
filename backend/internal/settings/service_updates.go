// Source: getarcaneapp/arcane/backend/internal/settings/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed APNS mobile push, template catalogue, GitOps, Swarm, standalone build workshop, OIDC and passkeys.

package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/florianfinn/docklet-next/types/settings"
	"go.getarcane.app/kit/normalization"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/httpx"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/validation"
)

func (s *SettingsService) UpdateSetting(ctx context.Context, key, value string) error {
	if err := validateSettingValueInternal(key, value); err != nil {
		return err
	}
	if err := libarcane.ValidateCronSetting(key, value); err != nil {
		return fmt.Errorf("invalid cron expression for %s: %w", key, err)
	}
	s.writes.Lock()
	defer s.writes.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := s.updateSettingValueNoRefreshInternal(ctx, key, value); err != nil {
		return err
	}
	return s.refreshSettingsCacheInternal(context.WithoutCancel(ctx))
}

// UpdateSettingValues persists a group of internal setting values atomically
// and publishes the refreshed snapshot before returning.
func (s *SettingsService) UpdateSettingValues(ctx context.Context, updates []libarcane.SettingUpdate) error {
	values := make([]SettingVariable, 0, len(updates))
	for _, update := range updates {
		if err := validateSettingValueInternal(update.Key, update.Value); err != nil {
			return err
		}
		values = append(values, SettingVariable{Key: update.Key, Value: update.Value})
	}
	s.writes.Lock()
	defer s.writes.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := s.persistSettings(ctx, values); err != nil {
		return err
	}
	return s.refreshSettingsCacheInternal(context.WithoutCancel(ctx))
}

func (s *SettingsService) updateSettingValueNoRefreshInternal(ctx context.Context, key, value string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		settingVar := &SettingVariable{
			Key:   key,
			Value: value,
		}
		return tx.Save(settingVar).Error
	})
}

// UpdateSettings publishes the refreshed snapshot before returning. Each
// subscriber's effects remain ordered and may finish after this method returns.
func (s *SettingsService) UpdateSettings(ctx context.Context, updates settings.Update) ([]SettingVariable, error) {
	if updates.AuthLocalEnabled != nil {
		return nil, common.Classify(common.ErrValidation, errors.New("sign-in mode is read-only; configure AUTH_MODE and restart"))
	}
	if err := normalization.Normalize(&updates); err != nil {
		return nil, err
	}
	s.writes.Lock()
	defer s.writes.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result, err := s.updateSettingsInternal(ctx, updates)
	if err == nil {
		s.publishSettingsChangesInternal(result.changes)
	}
	return result.settings, err
}

func (s *SettingsService) publishSettingsChangesInternal(updates []libarcane.SettingUpdate) {
	safe := make([]libarcane.SettingUpdate, 0, len(updates))
	cfg := s.GetSettingsConfig()
	for _, update := range updates {
		_, _, sensitive, err := cfg.FieldByKey(update.Key)
		if err != nil {
			slog.WarnContext(s.lifecycleCtx, "Skipping settings change notification for unknown key", "key", update.Key, "error", err)
			continue
		}
		if !sensitive {
			safe = append(safe, update)
		}
	}
	if len(safe) > 0 {
		s.changes.Publish(safe)
	}
}

func (s *SettingsService) updateSettingsInternal(ctx context.Context, updates settings.Update) (settingsUpdateResultInternal, error) {
	defaultCfg := s.getDefaultSettings()
	cfg := s.GetSettingsConfig().Clone()
	trivyServerTokenUpdated := updates.TrivyServerToken != nil && *updates.TrivyServerToken != "" && !s.IsEnvOverrideActive("trivyServerToken")
	normalizeTargetURL := func(value string) string {
		normalized, err := httpx.NormalizeBaseURL(value)
		if err != nil {
			return strings.TrimSpace(value)
		}
		return normalized
	}

	if err := validation.ValidateCredentialTargetChange(
		"Trivy server URL",
		cfg.TrivyServerUrl.Value,
		updates.TrivyServerUrl,
		normalizeTargetURL,
		map[string]bool{"trivyServerToken": cfg.TrivyServerToken.Value != ""},
		map[string]bool{"trivyServerToken": trivyServerTokenUpdated},
	); err != nil {
		return settingsUpdateResultInternal{}, err
	}

	valuesToUpdate, err := s.prepareUpdateValues(updates, cfg, defaultCfg)
	if err != nil {
		return settingsUpdateResultInternal{}, err
	}
	if trivyServerTokenUpdated {
		valuesToUpdate = append(valuesToUpdate, SettingVariable{Key: "trivyServerToken", Value: *updates.TrivyServerToken})
	}

	if persistSettingsErr := s.persistSettings(ctx, valuesToUpdate); persistSettingsErr != nil {
		return settingsUpdateResultInternal{}, persistSettingsErr
	}

	if refreshSettingsCacheErr := s.refreshSettingsCacheInternal(context.WithoutCancel(ctx)); refreshSettingsCacheErr != nil {
		return settingsUpdateResultInternal{}, refreshSettingsCacheErr
	}
	settingsCfg := s.GetSettingsConfig()
	changes := make([]libarcane.SettingUpdate, 0, len(valuesToUpdate))
	for _, value := range valuesToUpdate {
		changes = append(changes, libarcane.SettingUpdate{Key: value.Key, Value: value.Value})
	}
	return settingsUpdateResultInternal{
		settings: settingsCfg.ToSettingVariableSlice(SettingVisibilityNonAdmin, false),
		changes:  changes,
	}, nil
}

func (s *SettingsService) prepareUpdateValues(updates settings.Update, cfg, defaultCfg *Settings) ([]SettingVariable, error) {
	rt := reflect.TypeFor[settings.Update]()
	rv := reflect.ValueOf(updates)
	valuesToUpdate := make([]SettingVariable, 0)

	for i := range rt.NumField() {
		field := rt.Field(i)
		fieldValue := rv.Field(i)

		key, value, ok := extractUpdateValue(field, fieldValue)
		if !ok {
			continue
		}

		if err := validateSettingValueInternal(key, value); err != nil {
			return nil, err
		}

		if key == libarcane.DepotTokenSettingKey {
			// Sensitive token: only update when explicitly provided.
			// Empty input preserves existing token.
			if strings.TrimSpace(value) == "" {
				continue
			}

			if err := cfg.UpdateField(key, value, false); err != nil {
				return nil, fmt.Errorf("failed to update in-memory config for key '%s': %w", key, err)
			}

			valuesToUpdate = append(valuesToUpdate, SettingVariable{Key: key, Value: value})

			continue
		}

		if err := libarcane.ValidateCronSetting(key, value); err != nil {
			return nil, fmt.Errorf("invalid cron expression for %s: %w", key, err)
		}

		var valueToSave string
		var err error

		if value == "" {
			defaultValue, _, _, _ := defaultCfg.FieldByKey(key)
			valueToSave = defaultValue
			err = cfg.UpdateField(key, defaultValue, true)
		} else {
			valueToSave = value
			err = cfg.UpdateField(key, value, true)
		}

		if errors.Is(err, SettingSensitiveForbiddenError{}) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to update in-memory config for key '%s': %w", key, err)
		}

		valuesToUpdate = append(valuesToUpdate, SettingVariable{Key: key, Value: valueToSave})
	}

	return valuesToUpdate, nil
}

func extractUpdateValue(field reflect.StructField, fieldValue reflect.Value) (string, string, bool) {
	if fieldValue.Kind() == reflect.Pointer && fieldValue.IsNil() {
		return "", "", false
	}

	key, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if key == "" {
		return "", "", false
	}

	var value string
	if fieldValue.Kind() == reflect.Pointer {
		value = fieldValue.Elem().String()
	}

	return key, value, true
}

func (s *SettingsService) persistSettings(ctx context.Context, values []SettingVariable) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, setting := range values {
			if err := tx.Save(&setting).Error; err != nil {
				return fmt.Errorf("failed to update setting %s: %w", setting.Key, err)
			}
		}
		return nil
	})
}
