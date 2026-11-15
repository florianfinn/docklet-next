// Source: getarcaneapp/arcane/backend/pkg/libarcane/settings.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Functional cut: removed standalone build workshop and GitOps timeout key; M18 live scheduling, policy and contract foundation; M19 backend contracts and target identity integration; Arcane image registry host helpers removed (#235); docker client refresh interval validated as cron (#407).

package libarcane

import (
	"fmt"
	"slices"
	"strings"

	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/schedule"
)

const (
	DepotTokenSettingKey = "depotToken"
)

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
	"environmentHealthInterval",
	"eventCleanupInterval",
	"expiredSessionsCleanupInterval",
	"dockerClientRefreshInterval",
}

// TimeoutSettingKeys returns the settings propagated to remote environments.
func TimeoutSettingKeys() []string {
	return slices.Clone(timeoutSettingKeys)
}

// AgentSyncedSettingKeys returns a fresh copy of every hub setting propagated to agents.
func AgentSyncedSettingKeys() []string {
	return append(TimeoutSettingKeys(), "activityEventLimit", "activityRunLimit")
}

func IsCronSettingKey(key string) bool {
	return slices.Contains(cronSettingKeys, key)
}

func ValidateCronSetting(key, value string) error {
	if value == "" || !IsCronSettingKey(key) {
		return nil
	}
	// Cron settings take six explicit fields; descriptors such as @daily stay refused.
	if strings.HasPrefix(strings.TrimSpace(value), "@") {
		return fmt.Errorf("cron descriptor %q is not a six-field expression", value)
	}
	_, err := schedule.Parse(value, nil)
	return err
}
