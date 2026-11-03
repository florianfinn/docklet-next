// Source: getarcaneapp/arcane/backend/internal/notification/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Extract subscription defaults; M17 sustained-load opt-in preserves existing defaults.

package notification

import (
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
)

func (s *NotificationService) isEventEnabled(localConfig database.JSON, eventType notifications.NotificationEventType) bool {
	defaultEnabled := eventType != notifications.NotificationEventSustainedLoad
	events, ok := localConfig["events"].(map[string]any)
	if !ok {
		return defaultEnabled
	}

	enabled, ok := events[string(eventType)].(bool)
	if !ok {
		return defaultEnabled
	}

	return enabled
}
