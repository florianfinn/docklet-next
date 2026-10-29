// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package di

import (
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
)

func wireNotifications(n *notification.NotificationService, v *volume.VolumeService, s *system.SystemService) {
	v.SetNotificationDispatch(n.SendHostEvent)
	s.SetNotificationDispatch(n.SendHostEvent)
}
