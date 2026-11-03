// Source: docklet-next/issues/338
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original sustained-load opt-in preserving existing event defaults.

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
