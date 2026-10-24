// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

package image

import "strings"

// ValidateAgentUpgradeTarget uses the running hub version, never release-check
// metadata or a caller-supplied ceiling. Receivers separately reject blank input.
func ValidateAgentUpgradeTarget(target, hub string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return Failure("update.invalid-target", 400)
	}
	compared, ok := CompareRelease(target, strings.TrimSpace(hub))
	if !ok {
		return Failure("update.system-version-unknown", 503)
	}
	if compared > 0 {
		return Failure("update.requires-hub-update", 409)
	}
	return nil
}
