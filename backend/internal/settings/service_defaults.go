// Source: getarcaneapp/arcane/backend/internal/settings/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed APNS mobile push, template catalogue, GitOps, Swarm, standalone build workshop, OIDC and passkeys. #69 first-run setup and hub settings persistence; Explicit tools image overrides and shipped-default retirement; notification and Auto Heal contracts and delivery for #296.

package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"slices"
	"strings"

	"go.getarcane.app/kit/pkg"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
)

// DefaultSettingsConfig returns the canonical default settings model used by Arcane.
func DefaultSettingsConfig() *Settings {
	return &Settings{
		ProjectsDirectory:                     SettingVariable{Value: "/app/data/projects"},
		FollowProjectSymlinks:                 SettingVariable{Value: "false"},
		DiskUsagePath:                         SettingVariable{Value: "/app/data/projects"},
		AutoUpdate:                            SettingVariable{Value: "false"},
		AutoUpdateInterval:                    SettingVariable{Value: "0 0 0 * * *"},
		AutoUpdateExcludedContainers:          SettingVariable{Value: ""},
		PollingEnabled:                        SettingVariable{Value: "true"},
		PollingInterval:                       SettingVariable{Value: "0 0 * * * *"},
		ImageEventWatcherEnabled:              SettingVariable{Value: "false"},
		DockerClientRefreshInterval:           SettingVariable{Value: "0 */5 * * * *"},
		EventCleanupInterval:                  SettingVariable{Value: "0 0 */6 * * *"},
		ExpiredSessionsCleanupInterval:        SettingVariable{Value: "0 0 0 * * *"},
		UpgradeLogRetentionDays:               SettingVariable{Value: "3"},
		MaxConcurrentActivities:               SettingVariable{Value: "5"},
		KeepActionOutput:                      SettingVariable{Value: "true"},
		ActionPresentation:                    SettingVariable{Value: "dialog"},
		AutoInjectEnv:                         SettingVariable{Value: "false"},
		DefaultDeployPullPolicy:               SettingVariable{Value: "missing"},
		ToolsImage:                            SettingVariable{Value: ""},
		UpdateCheckRegistry:                   SettingVariable{Value: "auto"},
		ScheduledPruneEnabled:                 SettingVariable{Value: "false"},
		ScheduledPruneInterval:                SettingVariable{Value: "0 0 0 * * *"},
		PruneContainerMode:                    SettingVariable{Value: "stopped"},
		PruneContainerUntil:                   SettingVariable{Value: ""},
		PruneImageMode:                        SettingVariable{Value: "dangling"},
		PruneImageUntil:                       SettingVariable{Value: ""},
		PruneVolumeMode:                       SettingVariable{Value: "none"},
		PruneNetworkMode:                      SettingVariable{Value: "unused"},
		PruneNetworkUntil:                     SettingVariable{Value: ""},
		PruneBuildCacheMode:                   SettingVariable{Value: "none"},
		PruneBuildCacheUntil:                  SettingVariable{Value: ""},
		AutoHealExcludedStacks:                SettingVariable{Value: ""},
		AutoHealEnabled:                       SettingVariable{Value: "false"},
		AutoHealInterval:                      SettingVariable{Value: "0 */5 * * * *"},
		AutoHealExcludedContainers:            SettingVariable{Value: ""},
		AutoHealMaxRestarts:                   SettingVariable{Value: "5"},
		AutoHealRestartWindow:                 SettingVariable{Value: "30"},
		VolumeHelperIdleTimeout:               SettingVariable{Value: "10"},
		BaseServerURL:                         SettingVariable{Value: "http://localhost"},
		SetupCompleted:                        SettingVariable{Value: "false"},
		HubName:                               SettingVariable{Value: "Docklet"},
		HubAddress:                            SettingVariable{Value: ""},
		DomainURL:                             SettingVariable{Value: ""},
		LinkBase:                              SettingVariable{Value: "auto"},
		AgentProjectsDirectory:                SettingVariable{Value: "/home/docker"},
		AgentInstallDirectory:                 SettingVariable{Value: "/app/data/agent"},
		DefaultHostConnection:                 SettingVariable{Value: "direct"},
		DefaultPUID:                           SettingVariable{Value: ""},
		DefaultPGID:                           SettingVariable{Value: ""},
		DetectSocketGroup:                     SettingVariable{Value: "true"},
		EnableGravatar:                        SettingVariable{Value: "true"},
		ExperimentalFeaturesEnabled:           SettingVariable{Value: "false"},
		AvatarMaxUploadSizeMb:                 SettingVariable{Value: "2"},
		DefaultShell:                          SettingVariable{Value: "/bin/sh"},
		DockerHost:                            SettingVariable{Value: "unix:///var/run/docker.sock"},
		AuthLocalEnabled:                      SettingVariable{Value: "true"},
		AuthSessionTimeout:                    SettingVariable{Value: "1440"},
		AuthPasswordPolicy:                    SettingVariable{Value: "strong"},
		FeatureVulnerabilityManagementEnabled: SettingVariable{Value: "true"},
		VulnerabilityScanEnabled:              SettingVariable{Value: "false"},
		VulnerabilityScanSchedule:             SettingVariable{Value: ""},
		VulnerabilityNotifyCritical:           SettingVariable{Value: "false"},
		VulnerabilityNotifyHigh:               SettingVariable{Value: "false"},
		VulnerabilityNotifyFailed:             SettingVariable{Value: "false"},
		VulnerabilityScanInterval:             SettingVariable{Value: "0 0 0 * * *"},
		VulnerabilityThreatIntelEnabled:       SettingVariable{Value: "true"},
		TrivyDbRegistry:                       SettingVariable{Value: ""},
		TrivyNetwork:                          SettingVariable{Value: ""},
		TrivySecurityOpts:                     SettingVariable{Value: ""},
		TrivyPrivileged:                       SettingVariable{Value: "false"},
		TrivyResourceLimitsEnabled:            SettingVariable{Value: "true"},
		TrivyCpuLimit:                         SettingVariable{Value: "1"},
		TrivyMemoryLimitMb:                    SettingVariable{Value: "0"},
		TrivyConcurrentScanContainers:         SettingVariable{Value: "1"},
		TrivyServerEnabled:                    SettingVariable{Value: "false"},
		TrivyServerUrl:                        SettingVariable{Value: ""},
		TrivyServerToken:                      SettingVariable{Value: ""},
		TrivyIgnoreUnfixed:                    SettingVariable{Value: "true"},
		MaxImageUploadSize:                    SettingVariable{Value: "500"},
		EnvironmentHealthInterval:             SettingVariable{Value: "0 */2 * * * *"},
		LifecycleMaxTimeoutSec:                SettingVariable{Value: "300"},

		DockerAPITimeout:       SettingVariable{Value: "30"},
		DockerImagePullTimeout: SettingVariable{Value: "600"},
		TrivyScanTimeout:       SettingVariable{Value: "900"},
		ImagePatchSuffix:       SettingVariable{Value: "patched"},
		ImagePatchTimeoutSec:   SettingVariable{Value: "600"},
		ImagePatchAllPlatforms: SettingVariable{Value: "false"},
		ImageAutoPatchEnabled:  SettingVariable{Value: "false"},
		ImageAutoPatchInterval: SettingVariable{Value: "0 0 3 * * *"},
		HTTPClientTimeout:      SettingVariable{Value: "30"},
		RegistryTimeout:        SettingVariable{Value: "30"},
		RegistryTagTimeout:     SettingVariable{Value: "120"},
		ProxyRequestTimeout:    SettingVariable{Value: "60"},
		DeployWaitTimeout:      SettingVariable{Value: "600"},
		BuildProvider:          SettingVariable{Value: "local"},
		BuildTimeout:           SettingVariable{Value: "1800"},
		DepotProjectId:         SettingVariable{Value: ""},
		DepotToken:             SettingVariable{Value: ""},

		InstanceID: SettingVariable{Value: ""},
	}
}

// retiredSettingValuesInternal lists former defaults that startup replaces
// with the current default when an install still holds them verbatim.
var retiredSettingValuesInternal = map[string][]string{
	// Closed list: only values persisted before override-only storage. Bump PRs
	// must not extend it or include the current release default.
	"toolsImage":                  {"ghcr.io/getarcaneapp/tools:latest", "ghcr.io/florianfinn/docklet-tools:0.1.0"},
	"dockerClientRefreshInterval": {"*/30 * * * * *"},
	"autoHealInterval":            {"*/30 * * * * *"},
}

func (s *SettingsService) EnsureDefaultSettings(ctx context.Context) error {
	s.writes.Lock()
	defer s.writes.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	defaultSettings := s.getDefaultSettings()
	defaultSettingVars := defaultSettings.ToSettingVariableSlice(SettingVisibilityAll, false)

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, defaultSetting := range defaultSettingVars {
			var existing SettingVariable
			err := tx.Where("key = ?", defaultSetting.Key).First(&existing).Error

			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				if createDefaultSettingErr := tx.Create(&defaultSetting).Error; createDefaultSettingErr != nil {
					return fmt.Errorf("failed to create default setting %s: %w", defaultSetting.Key, createDefaultSettingErr)
				}
			case err != nil:
				return fmt.Errorf("failed to check for existing setting %s: %w", defaultSetting.Key, err)
			case slices.Contains(retiredSettingValuesInternal[defaultSetting.Key], existing.Value):
				if replaceDefaultSettingErr := tx.Model(&SettingVariable{}).Where("key = ?", defaultSetting.Key).Update("value", defaultSetting.Value).Error; replaceDefaultSettingErr != nil {
					return fmt.Errorf("failed to replace retired default for setting %s: %w", defaultSetting.Key, replaceDefaultSettingErr)
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (s *SettingsService) PruneUnknownSettings(ctx context.Context) error {
	s.writes.Lock()
	defer s.writes.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	allowedKeys := allowedSettingKeys()
	if len(allowedKeys) == 0 {
		return nil
	}

	keys := slices.Collect(maps.Keys(allowedKeys))

	result := s.db.WithContext(ctx).Where("key NOT IN ?", keys).Delete(&SettingVariable{})
	if result.Error != nil {
		return fmt.Errorf("failed to prune unknown settings: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		slog.InfoContext(ctx, "Pruned unknown settings", "count", result.RowsAffected)
	}
	return nil
}

func (s *SettingsService) PersistEnvSettingsIfMissing(ctx context.Context) error {
	s.writes.Lock()
	defer s.writes.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	rt := reflect.TypeFor[Settings]()
	appCfg := config.Load()
	isEnvOnlyMode := appCfg.AgentMode || appCfg.UIConfigurationDisabled

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for field := range rt.Fields() {
			if err := s.processEnvField(ctx, tx, field, isEnvOnlyMode); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return s.LoadDatabaseSettings(context.WithoutCancel(ctx))
}

func allowedSettingKeys() map[string]struct{} {
	allowed := make(map[string]struct{})

	settingsType := reflect.TypeFor[Settings]()
	for field := range settingsType.Fields() {
		key, _, _ := strings.Cut(field.Tag.Get("key"), ",")
		if key == "" {
			continue
		}
		allowed[key] = struct{}{}
	}

	allowed["encryptionKey"] = struct{}{}

	return allowed
}

func (s *SettingsService) processEnvField(ctx context.Context, tx *gorm.DB, field reflect.StructField, isEnvOnlyMode bool) error {
	tag := field.Tag.Get("key")
	key, attrs, _ := strings.Cut(tag, ",")

	if !s.shouldProcessField(key, attrs, isEnvOnlyMode) {
		return nil
	}

	envVarName := strings.ToUpper(kit.SnakeCase(key))
	envVal, ok, _ := utils.LookupEnvOrFile(envVarName)
	if !ok {
		return nil
	}
	envVal = kit.TrimQuotes(envVal)

	return s.upsertEnvSetting(ctx, tx, key, envVal)
}

func (s *SettingsService) shouldProcessField(key, attrs string, isEnvOnlyMode bool) bool {
	if key == "" || strings.Contains(attrs, "internal") {
		return false
	}

	// If not in env-only mode, only persist if it's explicitly marked as envOverride
	return isEnvOnlyMode || strings.Contains(attrs, "envOverride")
}
