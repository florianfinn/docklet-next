// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package system

import "github.com/florianfinn/docklet-next/backend/internal/notification/backupnotify"

func (s *SystemService) SetNotificationDispatch(dispatch backupnotify.Dispatch) {
	s.backup.SetNotificationDispatch(dispatch)
}
