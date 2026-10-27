// Source: getarcaneapp/arcane/backend/pkg/libarcane/settings.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Functional cut: removed standalone build workshop and GitOps timeout key.

package libarcane

import (
	"slices"
	"strings"

	"github.com/robfig/cron/v3"
	kit "go.getarcane.app/kit/pkg"
)

const (
	DepotTokenSettingKey = "depotToken"

	GHCRRegistryHost      = "ghcr.io"
	DockerHubRegistryHost = "docker.io"
	ECRPublicRegistryHost = "public.ecr.aws"
)

// ArcaneRegistryHost normalizes a registry setting to a host that mirrors the getarcaneapp images, defaulting to GHCR.
func ArcaneRegistryHost(registry string) string {
	return kit.Ternary(strings.TrimSpace(registry) == DockerHubRegistryHost, DockerHubRegistryHost, GHCRRegistryHost)
}

// ArcaneImageRegistryHost resolves the update-check registry setting; ECR Public only hosts the manager and agent images.
func ArcaneImageRegistryHost(registry string) string {
	if strings.TrimSpace(registry) == ECRPublicRegistryHost {
		return ECRPublicRegistryHost
	}
	return ArcaneRegistryHost(registry)
}

type SettingUpdate struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var timeoutSettingKeys = []string{
	"dockerApiTimeout",
	"dockerImagePullTimeout",
	"trivyScanTimeout",
	"httpClientTimeout",
	"registryTimeout",
	"registryTagTimeout",
	"proxyRequestTimeout",
	"deployWaitTimeout",
	"trivyResourceLimitsEnabled",
	"trivyCpuLimit",
	"trivyMemoryLimitMb",
	"buildProvider",
	"buildTimeout",
	"depotProjectId",
	DepotTokenSettingKey,
}

var cronSettingKeys = []string{
	"scheduledPruneInterval",
	"autoUpdateInterval",
	"pollingInterval",
	"environmentHealthInterval",
	"eventCleanupInterval",
	"expiredSessionsCleanupInterval",
}

var cronParser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// TimeoutSettingKeys returns the settings propagated to remote environments.
func TimeoutSettingKeys() []string {
	return slices.Clone(timeoutSettingKeys)
}

func IsCronSettingKey(key string) bool {
	return slices.Contains(cronSettingKeys, key)
}

func ValidateCronSetting(key, value string) error {
	if value == "" || !IsCronSettingKey(key) {
		return nil
	}
	_, err := cronParser.Parse(value)
	return err
}
