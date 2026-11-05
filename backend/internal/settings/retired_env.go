// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original startup warning for retired update and scan variables.

package settings

import (
	"context"
	"log/slog"

	"github.com/florianfinn/docklet-next/backend/pkg/utils"
)

// RetiredSettingVariables were replaced by the M18 update policy model and the scan
// schedule mode; a value set for them is ignored.
var RetiredSettingVariables = []string{"AUTO_UPDATE", "AUTO_UPDATE_INTERVAL", "POLLING_ENABLED", "POLLING_INTERVAL", "VULNERABILITY_SCAN_ENABLED"}

func retiredSettingVariablesInternal() []string {
	found := []string{}
	for _, name := range RetiredSettingVariables {
		if value, ok, _ := utils.LookupEnvOrFile(name); ok && value != "" {
			found = append(found, name)
		}
	}
	return found
}

func warnRetiredSettingVariablesInternal(ctx context.Context) {
	if found := retiredSettingVariablesInternal(); len(found) > 0 {
		slog.WarnContext(ctx, "Retired setting variables are ignored; configure updates and scans in Settings", "variables", found)
	}
}
