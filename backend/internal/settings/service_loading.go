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
	"slices"
	"strings"
	"time"

	"go.getarcane.app/kit/pkg"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
)

func (s *SettingsService) loadDatabaseSettingsInternal(ctx context.Context, db *database.DB) (*Settings, error) {
	if config.Load().UIConfigurationDisabled || config.Load().AgentMode {
		slog.DebugContext(
			ctx,
			"loadDatabaseSettingsInternal: using env path",
			"uiConfigurationDisabled",
			config.Load().UIConfigurationDisabled,
			"agentMode",
			config.Load().AgentMode,
			"environment",
			config.Load().Environment,
		)
		return s.loadDatabaseConfigFromEnv(ctx, db)
	}

	dest := s.getDefaultSettings()

	var loaded []SettingVariable
	queryCtx, queryCancel := context.WithTimeout(ctx, 10*time.Second)
	defer queryCancel()
	err := db.WithContext(queryCtx).
		Find(&loaded).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration from the database: %w", err)
	}

	for _, v := range loaded {
		err = dest.UpdateField(v.Key, v.Value, false)

		if err != nil && !errors.Is(err, SettingKeyNotFoundError{}) {
			return nil, fmt.Errorf("failed to process settings for key '%s': %w", v.Key, err)
		}
	}

	// Apply environment variable overrides for fields tagged with "envOverride"
	s.applyEnvOverrides(ctx, dest)

	return dest, nil
}

func (s *SettingsService) loadDatabaseConfigFromEnv(ctx context.Context, db *database.DB) (*Settings, error) {
	dest := s.getDefaultSettings()

	// Fetch all settings once to avoid N+1 queries for internal keys
	var allSettings []SettingVariable
	if err := db.WithContext(ctx).Find(&allSettings).Error; err != nil {
		return nil, fmt.Errorf("failed to load settings for env config: %w", err)
	}
	settingsMap := make(map[string]string, len(allSettings))
	for _, s := range allSettings {
		settingsMap[s.Key] = s.Value
	}

	rt := reflect.ValueOf(dest).Elem().Type()
	rv := reflect.ValueOf(dest).Elem()
	for i := range rt.NumField() {
		field := rt.Field(i)

		tagParts := strings.Split(field.Tag.Get("key"), ",")
		key := tagParts[0]
		isInternal := slices.Contains(tagParts[1:], "internal")

		if isInternal {
			if val, ok := settingsMap[key]; ok {
				rv.Field(i).FieldByName("Value").SetString(val)
			}
			continue
		}

		envVarName := strings.ToUpper(kit.SnakeCase(key))

		// debug: log each env name checked and whether a value exists
		// Match nonempty envOverride semantics: an empty tools image variable
		// must not mask an explicit value synchronised from the hub.
		if val, ok, _ := utils.LookupEnvOrFile(envVarName); ok && (key != "toolsImage" || val != "") {
			val = kit.TrimQuotes(val)
			mask := "<empty>"
			if val != "" {
				mask = fmt.Sprintf("%d chars", len(val))
			}
			slog.DebugContext(ctx, "loadDatabaseConfigFromEnv: env override found", "key", key, "env", envVarName, "valueMasked", mask)
			rv.Field(i).FieldByName("Value").SetString(val)
			continue
		}
		if val, ok := settingsMap[key]; ok {
			// Fallback to database if environment variable is not set
			slog.DebugContext(ctx, "loadDatabaseConfigFromEnv: using database fallback", "key", key)
			rv.Field(i).FieldByName("Value").SetString(val)
			continue
		}
		slog.DebugContext(ctx, "loadDatabaseConfigFromEnv: env not set and no database value", "key", key, "env", envVarName)
	}

	// debug: final snapshot (only show which fields are non-empty)
	count := 0
	for i := range rt.NumField() {
		v := rv.Field(i).FieldByName("Value").String()
		if v != "" {
			count++
		}
	}
	slog.DebugContext(ctx, "loadDatabaseConfigFromEnv: completed env load", "loadedFields", count)

	return dest, nil
}

func (s *SettingsService) applyEnvOverrides(ctx context.Context, dest *Settings) {
	_ = ctx
	rv := reflect.ValueOf(dest).Elem()

	for _, override := range s.envOverrides {
		rv.Field(override.fieldIndex).FieldByName("Value").SetString(override.value)
	}
}

func resolveSettingsEnvOverridesInternal() []settingsEnvOverride {
	rt := reflect.TypeFor[Settings]()
	overrides := make([]settingsEnvOverride, 0)

	for i := range rt.NumField() {
		field := rt.Field(i)
		tagValue := field.Tag.Get("key")
		if tagValue == "" {
			continue
		}

		// Parse tag attributes (e.g., "dockerHost,public,envOverride")
		parts := strings.Split(tagValue, ",")
		key := parts[0]
		hasEnvOverride := slices.Contains(parts[1:], "envOverride")

		if !hasEnvOverride {
			continue
		}

		envVarName := strings.ToUpper(kit.SnakeCase(key))
		if val, ok, _ := utils.LookupEnvOrFile(envVarName); ok && val != "" {
			overrides = append(overrides, settingsEnvOverride{
				fieldIndex: i,
				key:        key,
				envVarName: envVarName,
				value:      kit.TrimQuotes(val),
			})
		}
	}

	return overrides
}

// IsEnvOverrideActive reports whether an environment override currently controls the setting.
// and its corresponding environment variable is currently set to a non-empty value.
func (s *SettingsService) IsEnvOverrideActive(key string) bool {
	for _, override := range s.envOverrides {
		if override.key == key {
			return true
		}
	}

	return false
}
