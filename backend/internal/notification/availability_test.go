// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package notification

import (
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestAvailabilityClassesThresholdAndRestartReset(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}, &notificationoutbox.Availability{}))
	now := time.Now().UTC()
	for _, status := range []string{"online", "standby", "pending", "update-required", "token-rejected"} {
		require.NoError(t, s.observeHost(t.Context(), now, "remote", "Example", status, false))
	}
	var n int64
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, s.observeHost(t.Context(), now, "remote", "Example", "offline", false))
	require.NoError(t, s.observeHost(t.Context(), now.Add(4*time.Minute), "remote", "Example", "error", false))
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, s.ResetAvailability(t.Context(), now.Add(4*time.Minute)))
	require.NoError(t, s.observeHost(t.Context(), now.Add(6*time.Minute), "remote", "Example", "offline", false))
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.Zero(t, n, "restart resets the unnotified timer")
	require.NoError(t, s.observeHost(t.Context(), now.Add(10*time.Minute), "remote", "Example", "offline", false))
	require.NoError(t, s.observeHost(t.Context(), now.Add(11*time.Minute), "remote", "Example", "offline", false))
	require.NoError(t, s.observeHost(t.Context(), now.Add(12*time.Minute), "remote", "Example", "standby", false))
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.EqualValues(t, 2, n)
	require.NoError(t, s.observeHost(t.Context(), now, "gone", "Gone", "offline", false))
	require.NoError(t, s.observeHost(t.Context(), now.Add(time.Hour), "gone", "Gone", "offline", true))
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.EqualValues(t, 2, n)
}
func TestAvailabilityObserverUsesHostRuntimeProjection(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}, &notificationoutbox.Availability{}))
	now := time.Now().UTC()
	// A persisted Edge-online flag without a tunnel projects offline, not available.
	row := environment.Environment{ID: "remote", Name: "Example", Enabled: true, IsEdge: true, Status: "online", LastSeen: &now}
	require.NoError(t, db.Create(&row).Error)
	require.NoError(t, s.ObserveAvailability(t.Context(), now))
	require.NoError(t, s.ObserveAvailability(t.Context(), now.Add(6*time.Minute)))
	var n int64
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.EqualValues(t, 1, n)
	require.NoError(t, db.Delete(&row).Error)
	require.NoError(t, s.ObserveAvailability(t.Context(), now.Add(7*time.Minute)))
	require.NoError(t, db.Model(&notificationoutbox.Availability{}).Count(&n).Error)
	require.Zero(t, n)
}

func TestAvailabilityReadsPollStandbyAndDirectHealthWriter(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}, &notificationoutbox.Availability{}))
	now := time.Now().UTC()
	poll := environment.Environment{ID: "poll-only", Name: "Poll host", Enabled: true, IsEdge: true, Status: "offline", LastSeen: &now}
	require.NoError(t, db.Create(&poll).Error)
	edge.GetPollRuntimeRegistry().Update(poll.ID, time.Hour, now)
	require.NoError(t, s.ObserveAvailability(t.Context(), now))
	require.NoError(t, s.ObserveAvailability(t.Context(), now.Add(6*time.Minute)))
	var n int64
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.Zero(t, n, "poll-only standby is available")
	direct := environment.Environment{ID: "direct", Name: "Direct host", Enabled: true, Status: "online", LastSeen: &now}
	require.NoError(t, db.Create(&direct).Error)
	require.NoError(t, s.environmentSvc.UpdateEnvironmentConnectionState(t.Context(), direct.ID, false))
	require.NoError(t, s.ObserveAvailability(t.Context(), now))
	require.NoError(t, s.ObserveAvailability(t.Context(), now.Add(4*time.Minute)))
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, s.ObserveAvailability(t.Context(), now.Add(6*time.Minute)))
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.EqualValues(t, 1, n)
	require.NoError(t, db.Model(&direct).Update("status", "online").Error)
	require.NoError(t, s.ObserveAvailability(t.Context(), now.Add(7*time.Minute)))
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.EqualValues(t, 2, n)
}
