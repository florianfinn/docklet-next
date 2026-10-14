// Source: getarcaneapp/arcane/backend/internal/config/schema/schema_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed upstream analytics heartbeat and APNS mobile push; functional cut: removed template catalogue; functional cut: removed GitOps and saved Git repositories; functional cut: removed Swarm; functional cut: removed standalone build workshop; functional cut: removed OIDC and passkeys.; Removed downgrade setting from the configuration contract. Count-based history settings contract.

package schema

import (
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kit "go.getarcane.app/kit/pkg"

	"github.com/florianfinn/docklet-next/backend/internal/settings"
)

func TestGenerate_EnvConfigIncludesTaggedFields(t *testing.T) {
	doc, err := GenerateWithSourceRoot("")
	require.NoError(t, err)

	assert.Equal(t, expectedEnvConfigVars, envNames(doc.EnvConfig))

	entries := mapEnvEntries(doc.EnvConfig)

	appURL, ok := entries["APP_URL"]
	require.True(t, ok)
	assert.Equal(t, "AppUrl", appURL.Field)
	assert.Equal(t, "string", appURL.Type)
	assert.Equal(t, "http://localhost:3552", appURL.DefaultValue)
	assert.Equal(t, "config.Config", appURL.Source)
	assert.Equal(t, sourceFileConfig, appURL.SourceFile)

	encryptionKey, ok := entries["ENCRYPTION_KEY"]
	require.True(t, ok)
	assert.True(t, encryptionKey.SupportsFile)
	assert.Contains(t, encryptionKey.Options, "file")

	dockerConfig, ok := entries["DOCKER_CONFIG"]
	require.True(t, ok)
	assert.Equal(t, "DockerConfig", dockerConfig.Field)
	assert.Equal(t, "string", dockerConfig.Type)
	assert.Empty(t, dockerConfig.DefaultValue)
	assert.Equal(t, "config.Config", dockerConfig.Source)

	buildableUsername, ok := entries["AUTO_LOGIN_USERNAME"]
	require.True(t, ok)
	assert.True(t, buildableUsername.Conditional)
	assert.Equal(t, []string{"buildables"}, buildableUsername.BuildTags)
	assert.Equal(t, sourceFileBuildablesConfig, buildableUsername.SourceFile)
	assert.Equal(t, "config.BuildablesConfig", buildableUsername.Source)
}

func TestGenerate_SettingEnvOverridesMatchModelMetadata(t *testing.T) {
	doc, err := GenerateWithSourceRoot("")
	require.NoError(t, err)

	expectedCount := countDocumentedSettingOverrides()
	assert.Len(t, doc.SettingEnvOverrides, expectedCount)
	assert.Equal(t, expectedSettingOverrideKeys, sortedSettingKeys(doc.SettingEnvOverrides))

	entries := mapSettingOverrideEntries(doc.SettingEnvOverrides)
	feature, ok := entries["featureVulnerabilityManagementEnabled"]
	require.True(t, ok)
	assert.Equal(t, "FEATURE_VULNERABILITY_MANAGEMENT_ENABLED", feature.Env)
	assert.Equal(t, "true", feature.DefaultValue)
	assert.Equal(t, "boolean", feature.Type)
	assert.True(t, feature.Public)

	dockerTimeout, ok := entries["dockerApiTimeout"]
	require.True(t, ok)
	assert.Equal(t, "DOCKER_API_TIMEOUT", dockerTimeout.Env)
	assert.Equal(t, "30", dockerTimeout.DefaultValue)
	assert.Equal(t, "number", dockerTimeout.Type)
	assert.Equal(t, "timeouts", dockerTimeout.Category)
	assert.Equal(t, "settings.Settings + settings.DefaultSettingsConfig", dockerTimeout.Source)

	_, hasRuntimeDerived := entries["uiConfigDisabled"]
	assert.False(t, hasRuntimeDerived)
}

func TestGenerate_DocumentIncludesEnvOnlyModeNote(t *testing.T) {
	doc, err := GenerateWithSourceRoot("")
	require.NoError(t, err)

	notes := strings.Join(doc.Notes, " ")
	assert.Contains(t, notes, "AGENT_MODE=true")
	assert.Contains(t, notes, "UI_CONFIGURATION_DISABLED=true")
}

func TestGenerate_OutputOrderingIsStable(t *testing.T) {
	doc, err := GenerateWithSourceRoot("")
	require.NoError(t, err)

	assert.Equal(t, envNames(doc.EnvConfig), sortedStrings(envNames(doc.EnvConfig)))
	assert.Equal(t, categoriesThenEnv(doc.SettingEnvOverrides), sortedCategoryEnvPairs(doc.SettingEnvOverrides))
}

func mapEnvEntries(entries []ConfigEntry) map[string]ConfigEntry {
	result := make(map[string]ConfigEntry, len(entries))
	for _, entry := range entries {
		result[entry.Env] = entry
	}
	return result
}

func mapSettingOverrideEntries(entries []SettingOverrideEntry) map[string]SettingOverrideEntry {
	result := make(map[string]SettingOverrideEntry, len(entries))
	for _, entry := range entries {
		result[entry.SettingKey] = entry
	}
	return result
}

func envNames(entries []ConfigEntry) []string {
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.Env)
	}
	return result
}

func sortedSettingKeys(entries []SettingOverrideEntry) []string {
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.SettingKey)
	}
	sort.Strings(result)
	return result
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func categoriesThenEnv(entries []SettingOverrideEntry) []string {
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.Category+"|"+entry.Env)
	}
	return result
}

func sortedCategoryEnvPairs(entries []SettingOverrideEntry) []string {
	copied := append([]SettingOverrideEntry(nil), entries...)
	sort.Slice(copied, func(i, j int) bool {
		if copied[i].Category == copied[j].Category {
			return copied[i].Env < copied[j].Env
		}
		return copied[i].Category < copied[j].Category
	})

	result := make([]string, 0, len(copied))
	for _, entry := range copied {
		result = append(result, entry.Category+"|"+entry.Env)
	}
	return result
}

func countDocumentedSettingOverrides() int {
	settingsType := reflect.TypeFor[settings.Settings]()
	count := 0

	for field := range settingsType.Fields() {
		keyTag := field.Tag.Get("key")
		if keyTag == "" {
			continue
		}

		tagParts := kit.TrimNonEmpty(strings.Split(keyTag, ","))
		if len(tagParts) == 0 {
			continue
		}

		attrs := tagParts[1:]
		if slices.Contains(attrs, "envOverride") || !slices.Contains(attrs, "internal") {
			count++
		}
	}

	return count
}

var expectedEnvConfigVars = []string{
	"ACTOR_PORT",
	"ADMIN_STATIC_API_KEY",
	"AGENT_MODE",
	"AGENT_TOKEN",
	"ALLOW_CLI_PASSWORD_RESET",
	"APP_URL",
	"ARCANE_BACKUP_VOLUME_NAME",
	"AUTO_LOGIN_PASSWORD",
	"AUTO_LOGIN_USERNAME",
	"DATABASE_URL",
	"DIR_PERM",
	"DOCKER_CONFIG",
	"DOCKER_HOST",
	"EDGE_AGENT",
	"EDGE_MTLS_ASSETS_DIR",
	"EDGE_MTLS_CA_FILE",
	"EDGE_MTLS_CERT_FILE",
	"EDGE_MTLS_KEY_FILE",
	"EDGE_MTLS_MODE",
	"EDGE_MTLS_SERVER_NAME",
	"EDGE_RECONNECT_INTERVAL",
	"EDGE_TRANSPORT",
	"ENCRYPTION_KEY",
	"ENVIRONMENT",
	"FILE_PERM",
	"GPU_MONITORING_ENABLED",
	"GPU_TYPE",
	"HTTPS_PROXY",
	"HTTP_CLIENT_TIMEOUT",
	"HTTP_PROXY",
	"JWT_REFRESH_EXPIRY",
	"JWT_SECRET",
	"LISTEN",
	"LOG_JSON",
	"LOG_LEVEL",
	"MANAGER_API_URL",
	"NO_PROXY",
	"PGID",
	"PORT",
	"PROJECTS_DIRECTORY",
	"PROJECT_SCAN_MAX_DEPTH",
	"PROJECT_SCAN_SKIP_DIRS",
	"PROJECT_WORKSPACE_MAX_DEPTH",
	"PROJECT_WORKSPACE_MAX_ENTRIES",
	"PROJECT_WORKSPACE_MAX_FILE_SIZE_MB",
	"PUID",
	"TLS_CERT_FILE",
	"TLS_ENABLED",
	"TLS_KEY_FILE",
	"TRUSTED_PROXIES",
	"TZ",
	"UI_CONFIGURATION_DISABLED",
	"UPDATE_CHECK_DISABLED",
	"VOLUME_WORKSPACE_MAX_DEPTH",
	"VOLUME_WORKSPACE_MAX_ENTRIES",
	"VOLUME_WORKSPACE_MAX_FILE_SIZE_MB",
}

var expectedSettingOverrideKeys = []string{
	"authLocalEnabled",
	"authPasswordPolicy",
	"authSessionTimeout",
	"autoHealEnabled",
	"autoHealExcludedContainers",
	"autoHealInterval",
	"autoHealMaxRestarts",
	"autoHealRestartWindow",
	"autoInjectEnv",
	"autoUpdate",
	"autoUpdateExcludedContainers",
	"autoUpdateInterval",
	"avatarMaxUploadSizeMb",
	"baseServerUrl",
	"buildProvider",
	"buildTimeout",
	"defaultDeployPullPolicy",
	"defaultShell",
	"deployWaitTimeout",
	"depotProjectId",
	"depotToken",
	"diskUsagePath",
	"dockerApiTimeout",
	"dockerClientRefreshInterval",
	"dockerHost",
	"dockerImagePullTimeout",
	"enableGravatar",
	"environmentHealthInterval",
	"eventCleanupInterval",
	"experimentalFeaturesEnabled",
	"expiredSessionsCleanupInterval",
	"featureVulnerabilityManagementEnabled",
	"followProjectSymlinks",
	"httpClientTimeout",
	"imageAutoPatchEnabled",
	"imageAutoPatchInterval",
	"imageEventWatcherEnabled",
	"imagePatchAllPlatforms",
	"imagePatchSuffix",
	"imagePatchTimeoutSec",
	"lifecycleMaxTimeoutSec",
	"maxConcurrentActivities",
	"maxImageUploadSize",
	"pollingEnabled",
	"pollingInterval",
	"projectsDirectory",
	"proxyRequestTimeout",
	"pruneBuildCacheMode",
	"pruneBuildCacheUntil",
	"pruneContainerMode",
	"pruneContainerUntil",
	"pruneImageMode",
	"pruneImageUntil",
	"pruneNetworkMode",
	"pruneNetworkUntil",
	"pruneVolumeMode",
	"registryTagTimeout",
	"registryTimeout",
	"scheduledPruneEnabled",
	"scheduledPruneInterval",
	"toolsImageRegistry",
	"trivyConcurrentScanContainers",
	"trivyConfig",
	"trivyCpuLimit",
	"trivyDbRegistry",
	"trivyIgnore",
	"trivyIgnoreUnfixed",
	"trivyMemoryLimitMb",
	"trivyNetwork",
	"trivyPrivileged",
	"trivyResourceLimitsEnabled",
	"trivyScanTimeout",
	"trivySecurityOpts",
	"trivyServerEnabled",
	"trivyServerToken",
	"trivyServerUrl",
	"updateCheckRegistry",
	"upgradeLogRetentionDays",
	"volumeHelperIdleTimeout",
	"vulnerabilityScanEnabled",
	"vulnerabilityScanInterval",
	"vulnerabilityThreatIntelEnabled",
}
