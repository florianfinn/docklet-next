// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal settings descriptor rights.

package activity

import (
	"strings"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/capability"
)

var settingsOperationIDs = []string{
	"listNotificationTargets", "saveNotificationTarget", "deleteNotificationTarget", "testNotificationTarget",
	"getAutoHealSettings", "saveAutoHealSettings",
}

// Notification targets are hub-global and Auto Heal settings belong to one host; both
// route families admit only the global administrator.
func settingsCapabilityAllowedInternal(operation string, ref capability.ResourceRef, ps *authz.PermissionSet) bool {
	if !ps.IsGlobalAdmin() {
		return false
	}
	if operation == "getAutoHealSettings" || operation == "saveAutoHealSettings" {
		return ref.Kind == "host" && ref.ID != "" && !strings.Contains(ref.ID, "~")
	}
	return ref.Kind == "hub" && ref.ID == "0"
}
