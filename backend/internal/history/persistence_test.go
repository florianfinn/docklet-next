// Source: docklet-next/issues/352
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original persistence, duration, quiet-period and revision regression guards.

package history

import (
	dto "github.com/florianfinn/docklet-next/types/history"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func noticeCount(t *testing.T, s *Service) int64 {
	t.Helper()
	var n int64
	require.NoError(t, s.db.Table("notification_outbox").Where("event='container_unhealthy'").Count(&n).Error)
	return n
}
func TestM19SeedBeforeClassificationAndInspectionGap(t *testing.T) {
	s, host, now := samplerFixture(t)
	sample := fixtureSample()
	sample.Containers[0].Health = "unhealthy"
	require.NoError(t, s.commit(t.Context(), host, now, sample, nil))
	var before conditionRow
	require.NoError(t, s.db.Where("kind='container'").First(&before).Error)
	s = NewService(s.db, nil, nil, nil, nil)
	gap := fixtureSample()
	gap.Containers = nil
	gap.Uninspected = []string{"first"}
	require.NoError(t, s.commit(t.Context(), host, now.Add(7*time.Minute), gap, nil))
	for _, c := range s.snapshots.read(host.ID).Items {
		require.Equal(t, now, c.Since)
		if c.Kind == "container" {
			require.Equal(t, "app", c.Name)
			require.Equal(t, "project", c.ProjectID)
		}
	}
	var retained conditionRow
	require.NoError(t, s.db.Where("kind='container'").First(&retained).Error)
	require.Equal(t, before, retained)
	require.NoError(t, s.commit(t.Context(), host, now.Add(10*time.Minute), sample, nil))
	require.EqualValues(t, 1, countEvents(t, s, dto.EventUnhealthy))
	require.EqualValues(t, 1, noticeCount(t, s))
	absent := fixtureSample()
	absent.Containers = nil
	absent.Projects = nil
	require.NoError(t, s.commit(t.Context(), host, now.Add(11*time.Minute), absent, nil))
	var n int64
	require.NoError(t, s.db.Model(&conditionRow{}).Count(&n).Error)
	require.Zero(t, n)
}
func TestM19MinimumDurationAndRestartAtMinuteSeven(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "continuous", true: "restart"}[restart], func(t *testing.T) {
			s, host, now := samplerFixture(t)
			sample := fixtureSample()
			sample.Containers[0].Health = "unhealthy"
			for _, minute := range []int{0, 7, 9, 10, 11} {
				if restart && minute == 7 {
					s = NewService(s.db, nil, nil, nil, nil)
				}
				require.NoError(t, s.commit(t.Context(), host, now.Add(time.Duration(minute)*time.Minute), sample, nil))
				want := int64(0)
				if minute >= 10 {
					want = 1
				}
				require.Equal(t, want, noticeCount(t, s), minute)
			}
			require.EqualValues(t, 1, countEvents(t, s, dto.EventUnhealthy))
		})
	}
}
func TestM19ReasonSwitchKeepsUnhealthyClock(t *testing.T) {
	s, host, now := samplerFixture(t)
	sample := fixtureSample()
	sample.Containers[0].Health = "unhealthy"
	require.NoError(t, s.commit(t.Context(), host, now, sample, nil))
	sample.Containers[0].State = "restarting"
	require.NoError(t, s.commit(t.Context(), host, now.Add(7*time.Minute), sample, nil))
	require.NoError(t, s.commit(t.Context(), host, now.Add(10*time.Minute), sample, nil))
	require.EqualValues(t, 1, noticeCount(t, s))
	var row conditionRow
	require.NoError(t, s.db.Where("kind='container'").First(&row).Error)
	require.Equal(t, now.Unix(), *row.UnhealthySince)
	var reason string
	require.NoError(t, s.db.Raw("SELECT json_extract(payload,'$.reason') FROM notification_outbox WHERE event='container_unhealthy'").Scan(&reason).Error)
	require.Equal(t, "restart-loop", reason)
}
func TestM19QuietTimestampSurvivesHealthySampleAndRestart(t *testing.T) {
	s, host, now := samplerFixture(t)
	sample := fixtureSample()
	sample.Containers[0].Health = "unhealthy"
	for _, minute := range []int{0, 10} {
		require.NoError(t, s.commit(t.Context(), host, now.Add(time.Duration(minute)*time.Minute), sample, nil))
	}
	require.EqualValues(t, 1, noticeCount(t, s))
	require.NoError(t, s.commit(t.Context(), host, now.Add(11*time.Minute), fixtureSample(), nil))
	s = NewService(s.db, nil, nil, nil, nil)
	for _, minute := range []int{12, 22, 69} {
		require.NoError(t, s.commit(t.Context(), host, now.Add(time.Duration(minute)*time.Minute), sample, nil))
		require.EqualValues(t, 1, noticeCount(t, s))
	}
	require.NoError(t, s.commit(t.Context(), host, now.Add(70*time.Minute), sample, nil))
	require.EqualValues(t, 2, noticeCount(t, s))
}
func TestM19CounterDerivedRestartLoopSurvivesProcessRestart(t *testing.T) {
	s, host, now := samplerFixture(t)
	sample := fixtureSample()
	require.NoError(t, s.commit(t.Context(), host, now, sample, nil))
	sample.Containers[0].RestartCount = 3
	require.NoError(t, s.commit(t.Context(), host, now.Add(time.Minute), sample, nil))
	s = NewService(s.db, nil, nil, nil, nil)
	require.NoError(t, s.commit(t.Context(), host, now.Add(2*time.Minute), sample, nil))
	require.Zero(t, countEvents(t, s, dto.EventRunningAgain))
	for _, c := range s.snapshots.read(host.ID).Items {
		require.Equal(t, dto.Unhealthy, c.Condition)
		require.Equal(t, now.Add(time.Minute), c.Since)
	}
}
func TestM19RevisionResetsEvidenceButKeepsQuietTimestamp(t *testing.T) {
	s, host, now := samplerFixture(t)
	sample := fixtureSample()
	sample.Containers[0].State = "restarting"
	for _, minute := range []int{0, 10} {
		require.NoError(t, s.commit(t.Context(), host, now.Add(time.Duration(minute)*time.Minute), sample, nil))
	}
	require.EqualValues(t, 1, noticeCount(t, s))
	host.CredentialRevision++
	require.NoError(t, s.db.Model(&host).Update("credential_revision", host.CredentialRevision).Error)
	require.NoError(t, s.commit(t.Context(), host, now.Add(11*time.Minute), sample, nil))
	for _, c := range s.snapshots.read(host.ID).Items {
		require.Equal(t, now.Add(11*time.Minute), c.Since)
	}
	require.NoError(t, s.commit(t.Context(), host, now.Add(21*time.Minute), sample, nil))
	require.EqualValues(t, 1, noticeCount(t, s))
	var row conditionRow
	require.NoError(t, s.db.Where("kind='container'").First(&row).Error)
	require.Equal(t, now.Add(10*time.Minute).Unix(), *row.UnhealthyNotifiedAt)
}
func TestM19ConditionsWriteOnlyOnChangeAndRetainUnknownHosts(t *testing.T) {
	s, host, now := samplerFixture(t)
	sample := fixtureSample()
	require.NoError(t, s.commit(t.Context(), host, now, sample, nil))
	require.NoError(t, s.db.Exec("CREATE TABLE condition_writes(n INTEGER)").Error)
	require.NoError(t, s.db.Exec("CREATE TRIGGER observe_conditions AFTER UPDATE ON history_conditions BEGIN INSERT INTO condition_writes VALUES(1); END").Error)
	require.NoError(t, s.commit(t.Context(), host, now.Add(time.Minute), sample, nil))
	var n int64
	require.NoError(t, s.db.Table("condition_writes").Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, s.db.Where("id=?", host.ID).Delete(&host).Error)
	require.NoError(t, s.Retain(t.Context(), now))
	require.NoError(t, s.db.Model(&conditionRow{}).Count(&n).Error)
	require.Zero(t, n)
}

func TestM19CounterLoopNoticeAfterContinuousTenMinutes(t *testing.T) {
	s, host, now := samplerFixture(t)
	sample := fixtureSample()
	for minute := range 12 {
		sample.Containers[0].RestartCount = minute * 3
		require.NoError(t, s.commit(t.Context(), host, now.Add(time.Duration(minute)*time.Minute), sample, nil))
		want := 0
		if minute == 11 {
			want = 1
		}
		require.EqualValues(t, want, noticeCount(t, s), minute)
	}
}
