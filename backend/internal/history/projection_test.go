// Source: docklet-next/issues/338
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original real hub event, activity and outbox projection assertions.

package history

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	dto "github.com/florianfinn/docklet-next/types/history"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestHB17CommittedEventMetadataAndLoadActivity(t *testing.T) {
	s, host, now := samplerFixture(t)
	setting, err := settings.NewSettingsService(t.Context(), s.db)
	require.NoError(t, err)
	defer setting.Stop(context.Background())
	s.settings = setting
	require.NoError(t, setting.SetStringSetting(t.Context(), "loadAlertMinutes", "5"))
	sample := fixtureSample()
	sample.Host.MemoryTotal = 100
	sample.Host.MemoryUsed = 99
	sample.Containers[0].Stats.MemoryLimit = 100
	sample.Containers[0].Stats.MemoryUsage = 99
	sample.Containers[0].Health = "unhealthy"
	for i := 0; i < 6; i++ {
		sample.Host.CPUBusy += 95
		sample.Host.CPUTotal += 100
		sample.Containers[0].Stats.CPUTotal += 95
		sample.Containers[0].Stats.CPUSystem += 100
		require.NoError(t, s.commit(t.Context(), host, now.Add(time.Duration(i)*time.Minute), sample, nil))
		sample.Containers[0].Health = "healthy"
	}
	var activities []activity.Activity
	require.NoError(t, s.db.Where("type='load_sustained'").Find(&activities).Error)
	require.Len(t, activities, 4)
	for _, a := range activities {
		require.Equal(t, "load.sustained", a.MessageCode)
		require.Equal(t, host.ID, a.EnvironmentID)
		require.Contains(t, []any{"cpu", "memory"}, a.Metadata["metric"])
	}
	var notices int64
	require.NoError(t, s.db.Table("notification_outbox").Where("event='sustained_load'").Count(&notices).Error)
	require.EqualValues(t, 4, notices)
	var events []event.Event
	require.NoError(t, s.db.Order("timestamp").Find(&events).Error)
	require.Len(t, events, 2)
	require.Equal(t, event.EventType(dto.EventUnhealthy), events[0].Type)
	require.Equal(t, event.EventType(dto.EventRunningAgain), events[1].Type)
	for _, e := range events {
		require.Equal(t, host.ID, *e.EnvironmentID)
		require.Equal(t, "app", *e.ResourceName)
		require.Equal(t, "app", e.Metadata["containerName"])
		require.Equal(t, host.ID, e.Metadata["hostId"])
		require.EqualValues(t, 0, e.Metadata["exitCode"])
		require.EqualValues(t, 0, e.Metadata["restarts"])
	}
}
