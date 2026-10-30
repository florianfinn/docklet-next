// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package volume

import "context"

func (s *VolumeService) HeldForAutoHeal(ctx context.Context, names []string) (bool, error) {
	return s.backup.HeldForAutoHeal(ctx, names)
}
