// Source: docklet-next/issues/302
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original heartbeat-to-outbox threshold regression test.

package notification

import (
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestAvailabilityStaleHeartbeatNotifiesOnceAfterFiveMinutes(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}, &notificationoutbox.Availability{}))
	id := "example-frozen-edge"
	tunnel := edge.NewAgentTunnelWithConn(id, edge.NewGRPCManagerTunnelConn(nil))
	edge.GetRegistry().Register(id, tunnel)
	defer edge.GetRegistry().Unregister(id)
	now := tunnel.GetLastHeartbeat()
	require.NoError(t, db.Create(&environment.Environment{ID: id, Name: "Frozen example", Enabled: true, IsEdge: true, Status: "online", LastSeen: &now}).Error)
	for _, offset := range []time.Duration{91 * time.Second, 91*time.Second + 4*time.Minute} {
		require.NoError(t, s.ObserveAvailability(t.Context(), now.Add(offset)))
	}
	var count int64
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&count).Error)
	require.Zero(t, count)
	for _, offset := range []time.Duration{91*time.Second + 5*time.Minute, 91*time.Second + 6*time.Minute} {
		require.NoError(t, s.ObserveAvailability(t.Context(), now.Add(offset)))
	}
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
