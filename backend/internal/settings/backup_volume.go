// Source: docklet-next/issues/570
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original read-only bridge to the volume-owned resolver.; M36 #570 scoped baseline, storage naming or address guidance.

package settings

import "context"

// SetBackupVolumeNameReader wires host-local volume metadata before routes serve requests.
func (s *SettingsService) SetBackupVolumeNameReader(read func(context.Context) (string, error)) {
	s.backupVolumeNameReader = read
}
