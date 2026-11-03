// Source: docklet-next/issues/338
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original real hub event, activity and outbox projection assertions.

package history

import (
	"context"
	"errors"
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

func countEvents(t *testing.T, s *Service, code string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, s.db.Model(&event.Event{}).Where("type=?", code).Count(&n).Error)
	return n
}

// A failed host minute is no transition: an unchanged condition is not reported again afterwards.
func TestConditionTransitionsSurviveFailedMinute(t *testing.T) {
	s, host, now := samplerFixture(t)
	down := fixtureSample()
	down.Containers[0].State, down.Containers[0].ExitCode, down.Containers[0].Stats = "exited", 1, nil
	minute := func(i int) time.Time { return now.Add(time.Duration(i) * time.Minute) }
	require.NoError(t, s.commit(t.Context(), host, minute(0), down, nil))
	since := s.snapshots.read(host.ID).Items[0].Since
	require.NoError(t, s.commit(t.Context(), host, minute(1), dto.Sample{}, errors.New("agent unreachable")))
	require.NoError(t, s.commit(t.Context(), host, minute(2), down, nil))
	require.EqualValues(t, 1, countEvents(t, s, dto.EventDown))
	for _, c := range s.snapshots.read(host.ID).Items {
		require.Equal(t, since, c.Since, c.Kind)
	}
	require.NoError(t, s.commit(t.Context(), host, minute(3), dto.Sample{}, errors.New("agent unreachable")))
	require.NoError(t, s.commit(t.Context(), host, minute(4), fixtureSample(), nil))
	require.EqualValues(t, 1, countEvents(t, s, dto.EventRunningAgain))
	require.EqualValues(t, 1, countEvents(t, s, dto.EventDown))
}

// One failed host minute keeps a container's alerted load episode: continuous load alerts once.
func TestLoadEpisodeSurvivesFailedMinute(t *testing.T) {
	s, host, now := samplerFixture(t)
	setting, err := settings.NewSettingsService(t.Context(), s.db)
	require.NoError(t, err)
	defer setting.Stop(context.Background())
	s.settings = setting
	require.NoError(t, setting.SetStringSetting(t.Context(), "loadAlertMinutes", "5"))
	sample := fixtureSample()
	minute := 0
	high := func(n int) {
		for i := 0; i < n; i++ {
			sample.Containers[0].Stats.CPUTotal += 95
			sample.Containers[0].Stats.CPUSystem += 100
			require.NoError(t, s.commit(t.Context(), host, now.Add(time.Duration(minute)*time.Minute), sample, nil))
			minute++
		}
	}
	high(8)
	require.NoError(t, s.commit(t.Context(), host, now.Add(time.Duration(minute)*time.Minute), dto.Sample{}, errors.New("agent unreachable")))
	minute++
	high(8)
	var alerts int64
	require.NoError(t, s.db.Model(&activity.Activity{}).Where("type='load_sustained' AND resource_type='container' AND metadata LIKE '%\"cpu\"%'").Count(&alerts).Error)
	require.EqualValues(t, 1, alerts)
}

// A container listed but not inspected for one minute is no transition and keeps its alerted episode.
func TestUninspectedContainerKeepsConditionAndEpisode(t *testing.T) {
	s, host, now := samplerFixture(t)
	setting, err := settings.NewSettingsService(t.Context(), s.db)
	require.NoError(t, err)
	defer setting.Stop(context.Background())
	s.settings = setting
	require.NoError(t, setting.SetStringSetting(t.Context(), "loadAlertMinutes", "5"))
	minute := 0
	commit := func(sample dto.Sample) {
		require.NoError(t, s.commit(t.Context(), host, now.Add(time.Duration(minute)*time.Minute), sample, nil))
		minute++
	}
	skipped := func(sample dto.Sample) dto.Sample {
		gap := cloneSample(sample)
		gap.Uninspected, gap.Containers = []string{sample.Containers[0].ID}, nil
		return *gap
	}
	down := fixtureSample()
	down.Containers[0].State, down.Containers[0].ExitCode, down.Containers[0].Stats = "exited", 1, nil
	commit(down)
	since := s.snapshots.read(host.ID).Items
	commit(skipped(down))
	require.Equal(t, since[0].Since, s.snapshots.read(host.ID).Items[0].Since)
	commit(down)
	require.EqualValues(t, 1, countEvents(t, s, dto.EventDown))
	require.Equal(t, since[0].Since, s.snapshots.read(host.ID).Items[0].Since)

	sample := fixtureSample()
	high := func(n int) {
		for i := 0; i < n; i++ {
			sample.Containers[0].Stats.CPUTotal += 95
			sample.Containers[0].Stats.CPUSystem += 100
			commit(sample)
		}
	}
	high(8)
	commit(skipped(sample))
	high(8)
	var alerts int64
	require.NoError(t, s.db.Model(&activity.Activity{}).Where("type='load_sustained' AND resource_type='container' AND metadata LIKE '%\"cpu\"%'").Count(&alerts).Error)
	require.EqualValues(t, 1, alerts)
}
