// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original history storage, classification and lifecycle break-map tests.

package history

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	dto "github.com/florianfinn/docklet-next/types/history"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func historyDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "history.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}
func fixtureSample() dto.Sample {
	return dto.Sample{Host: dto.HostSample{MemoryUsed: 500, MemoryTotal: 1000, DiskUsed: 10, DiskTotal: 100, DiskSource: "/example", CPUBusy: 10, CPUTotal: 100}, Containers: []dto.ContainerSample{{ID: "first", Name: "app", ProjectID: "project", State: "running", RestartCount: 0, Stats: &dto.Counters{CPUTotal: 10, CPUSystem: 100, MemoryUsage: 400, MemoryLimit: 500}}}, Projects: []dto.ProjectSample{{ID: "project", Name: "example", Status: "running"}}}
}
func samplerFixture(t *testing.T) (*Service, environment.Environment, time.Time) {
	db := historyDB(t)
	host := environment.Environment{ID: "host", Name: "Example", ApiUrl: "https://agent.example.com", Enabled: true}
	require.NoError(t, db.Create(&host).Error)
	s := NewService(db, nil, nil, nil, nil)
	return s, host, time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
}

func TestHB1RepeatedMinuteAndLaterMinuteAcrossReopen(t *testing.T) {
	path := sqlitestore.URL(filepath.Join(t.TempDir(), "history.db"))
	db, err := database.Initialize(t.Context(), path, database.MigrationOptions{})
	require.NoError(t, err)
	minute := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC).Unix()
	o := observation{kind: "container", key: "app", state: dto.Running, cpu: new(20.)}
	write := func(m int64) {
		require.NoError(t, db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error { return upsertMinute(tx, "host", m, o) }))
	}
	write(minute)
	write(minute)
	write(minute + 60)
	require.NoError(t, db.Close())
	db, err = database.Initialize(t.Context(), path, database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	rows := []bucketRow{}
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 3)
	for _, r := range rows {
		require.Equal(t, 2, r.MinutesRunning)
		require.Equal(t, 2, r.CPUN)
		require.Equal(t, 40., r.CPUSum)
	}
}
func TestHB2CounterResetReplacementAndFirstSample(t *testing.T) {
	s, host, now := samplerFixture(t)
	sample := fixtureSample()
	next := hostState{restarts: map[string][]restartPoint{}}
	old := hostState{}
	observations, _ := s.observations(host.ID, now, sample, old, &next)
	require.Nil(t, observations[1].cpu)
	require.Zero(t, observations[1].rx)
	previous := fixtureSample()
	sample.Containers[0].Stats.CPUTotal = 30
	sample.Containers[0].Stats.CPUSystem = 200
	sample.Containers[0].Stats.NetworkRx = 100
	old.previous = &previous
	next.restarts = map[string][]restartPoint{}
	observations, _ = s.observations(host.ID, now, sample, old, &next)
	require.Equal(t, 20., *observations[1].cpu)
	require.Equal(t, 100., observations[1].rx)
	sample.Containers[0].ID = "replacement"
	observations, _ = s.observations(host.ID, now, sample, old, &next)
	require.Nil(t, observations[1].cpu)
	require.Zero(t, observations[1].rx)
	sample.Containers[0].ID = "first"
	sample.Containers[0].Stats.CPUTotal = 1
	observations, _ = s.observations(host.ID, now, sample, old, &next)
	require.Nil(t, observations[1].cpu)
	old.previous = cloneSample(sample)
	sample.Containers[0].Stats.CPUTotal = 21
	sample.Containers[0].Stats.CPUSystem = 300
	observations, _ = s.observations(host.ID, now.Add(time.Minute), sample, old, &next)
	require.Equal(t, 20., *observations[1].cpu)
}
func TestHB3FailurePartialStatsAndRecovery(t *testing.T) {
	s, host, now := samplerFixture(t)
	sample := fixtureSample()
	sample.Containers[0].Stats = nil
	sample.Containers[0].StatsMissing = true
	require.NoError(t, s.commit(t.Context(), host, now, sample, nil))
	var c bucketRow
	require.NoError(t, s.db.Where("kind='container' AND tier=15").Take(&c).Error)
	require.Equal(t, 1, c.MinutesRunning)
	require.Zero(t, c.CPUN)
	require.NoError(t, s.commit(t.Context(), host, now.Add(time.Minute), dto.Sample{}, errors.New("offline")))
	require.NoError(t, s.db.Where("kind='container' AND tier=15").Take(&c).Error)
	require.Equal(t, 1, c.MinutesRunning)
	var h bucketRow
	require.NoError(t, s.db.Where("kind='host' AND tier=15").Take(&h).Error)
	require.Equal(t, 1, h.MinutesDown)
	sample = fixtureSample()
	require.NoError(t, s.commit(t.Context(), host, now.Add(2*time.Minute), sample, nil))
	require.NoError(t, s.db.Where("kind='container' AND tier=15").Take(&c).Error)
	require.Equal(t, 2, c.MinutesRunning)
	require.Zero(t, c.CPUN)
	e := episode{}
	for i := 0; i < 4; i++ {
		e, _ = evaluateLoad(e, int64(i*60), new(99.), 90, 5)
	}
	e, _ = evaluateLoad(e, 240, nil, 90, 5)
	_, alert := evaluateLoad(e, 300, new(99.), 90, 5)
	require.False(t, alert)
}
func TestHB4ContainerNameContinuity(t *testing.T) {
	s, host, now := samplerFixture(t)
	sample := fixtureSample()
	require.NoError(t, s.commit(t.Context(), host, now, sample, nil))
	sample.Containers[0].ID = "replacement"
	require.NoError(t, s.commit(t.Context(), host, now.Add(time.Minute), sample, nil))
	sample.Containers[0].Name = "renamed"
	require.NoError(t, s.commit(t.Context(), host, now.Add(2*time.Minute), sample, nil))
	var rows []bucketRow
	require.NoError(t, s.db.Where("kind='container' AND tier=15").Order("entity_key").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, "app", rows[0].EntityKey)
	require.Equal(t, 2, rows[0].MinutesRunning)
	require.Equal(t, "renamed", rows[1].EntityKey)
}
func TestHB6ConditionPrecedenceAndRestartWindow(t *testing.T) {
	for _, tc := range []struct {
		state, health string
		exit          int
		intent, oom   bool
		loop          int
		want          dto.State
		reason        string
	}{
		{"restarting", "", 0, false, false, 0, dto.Unhealthy, "restart-loop"}, {"running", "unhealthy", 0, false, false, 3, dto.Unhealthy, "restart-loop"}, {"running", "unhealthy", 0, false, false, 0, dto.Unhealthy, "health"}, {"running", "starting", 0, false, false, 0, dto.Running, ""}, {"paused", "", 0, false, false, 9, dto.Stopped, "paused"}, {"exited", "", 137, true, true, 9, dto.Stopped, "intent"}, {"exited", "", 1, false, false, 9, dto.Down, "exit"}, {"exited", "", 0, false, true, 9, dto.Down, "oom"}, {"exited", "", 0, false, false, 9, dto.Stopped, "operator-stop"}, {"exited", "", 143, false, false, 9, dto.Stopped, "operator-stop"}, {"created", "", 1, false, false, 9, dto.Stopped, "never-started"}, {"dead", "", 0, false, false, 9, dto.Down, "exit"},
	} {
		state, reason := Classify(dto.ContainerSample{State: tc.state, Health: tc.health, ExitCode: tc.exit, StopIntent: tc.intent, OOMKilled: tc.oom}, tc.loop)
		require.Equal(t, tc.want, state)
		require.Equal(t, tc.reason, reason)
	}
	p, n := restartWindow(nil, 0, 1)
	p, n = restartWindow(p, 60, 4)
	require.Equal(t, 3, n)
	_, n = restartWindow(p, 660, 4)
	require.Zero(t, n)
	_, n = restartWindow(p, 120, 0)
	require.Zero(t, n)
	require.Equal(t, dto.Running, stackState([]dto.Condition{{Condition: dto.Running}, {Condition: dto.Stopped}}, ""))
	require.Equal(t, dto.Down, stackState([]dto.Condition{{Condition: dto.Down}, {Condition: dto.Unhealthy}}, ""))
	require.Equal(t, dto.Stopped, stackState(nil, "stopped"))
}
func TestHB7TiersSlotsAndRetentionEdges(t *testing.T) {
	db := historyDB(t)
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return upsertMinute(tx, "host", now.Unix(), observation{kind: "host", key: "host", state: dto.Running})
	}))
	for value, count := range map[string]int{"24h": 96, "7d": 84, "30d": 90} {
		r, err := readHistory(t.Context(), db, "host", "host", "host", value, now)
		require.NoError(t, err)
		require.Len(t, r.Buckets, count)
		require.True(t, r.Buckets[count-1].Partial)
		require.Equal(t, dto.Running, r.Buckets[count-1].State)
		require.Equal(t, dto.NoData, r.Buckets[0].State)
	}
	for tier, hours := range map[int]int{15: 24, 120: 168, 480: 720} {
		edge := now.Add(-time.Duration(hours) * time.Hour).Unix()
		for _, stamp := range []int64{edge - 1, edge} {
			require.NoError(t, db.Create(&bucketRow{Kind: "host", HostID: "retention", EntityKey: "retention", Tier: tier, BucketStart: stamp, LastMinute: stamp}).Error)
		}
	}
	require.NoError(t, retain(t.Context(), db, now))
	var rows []bucketRow
	require.NoError(t, db.Where("host_id='retention'").Find(&rows).Error)
	require.Len(t, rows, 3)
}
func TestHB8SampleValidation(t *testing.T) {
	for _, mutate := range []func(*dto.Sample){func(s *dto.Sample) { s.Containers = append(s.Containers, s.Containers[0]) }, func(s *dto.Sample) { s.Host.CPUBusy = math.NaN() }, func(s *dto.Sample) { s.Containers[0].State = "invented" }, func(s *dto.Sample) { s.Host.DiskUsed = -1 }, func(s *dto.Sample) { s.Projects = append(s.Projects, s.Projects[0]) }} {
		s := fixtureSample()
		mutate(&s)
		require.EqualError(t, Validate(s), "history.sample-invalid")
	}
	require.NoError(t, Validate(fixtureSample()))
}
func TestHB11SnapshotIsolation(t *testing.T) {
	holder := snapshots{}
	original := dto.Conditions{Items: []dto.Condition{{Name: "app", Condition: dto.Down}}}
	holder.publish("host", original)
	original.Items[0].Name = "input mutation"
	first := holder.read("host")
	first.Items[0].Condition = dto.Stopped
	first.Items = append(first.Items, dto.Condition{Name: "injected"})
	second := holder.read("host")
	require.Len(t, second.Items, 1)
	require.Equal(t, "app", second.Items[0].Name)
	require.Equal(t, dto.Down, second.Items[0].Condition)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Go(func() {
			for n := 0; n < 50; n++ {
				holder.publish("host", dto.Conditions{Items: []dto.Condition{{Name: "app"}}})
				read := holder.read("host")
				read.Items[0].Name = "caller"
			}
		})
	}
	wg.Wait()
	require.Equal(t, "app", holder.read("host").Items[0].Name)
}
func TestHB12CommitFenceOverlapAndCancellation(t *testing.T) {
	t.Run("publication-gate", func(t *testing.T) {
		s, host, now := samplerFixture(t)
		s.beforePublish = func() {
			require.NoError(t, s.db.Model(&environment.Environment{}).Where("id=?", host.ID).Update("credential_revision", 1).Error)
		}
		require.NoError(t, s.commit(t.Context(), host, now, fixtureSample(), nil))
		require.Empty(t, s.states)
		require.Empty(t, s.snapshots.read(host.ID).Items)
	})
	t.Run("older-minute", func(t *testing.T) {
		s, host, now := samplerFixture(t)
		sample := fixtureSample()
		require.NoError(t, s.commit(t.Context(), host, now, sample, nil))
		before := s.snapshots.read(host.ID)
		require.NoError(t, s.commit(t.Context(), host, now.Add(-time.Minute), sample, nil))
		require.Equal(t, before, s.snapshots.read(host.ID))
		require.Equal(t, now.Unix(), s.states[host.ID].minute)
	})
	for _, change := range []string{"disable", "revision", "endpoint", "remove"} {
		t.Run(change, func(t *testing.T) {
			s, host, now := samplerFixture(t)
			entered, release := make(chan struct{}), make(chan struct{})
			s.fetch = func(ctx context.Context, _ environment.Environment) (dto.Sample, error) {
				close(entered)
				select {
				case <-release:
					return fixtureSample(), nil
				case <-ctx.Done():
					return dto.Sample{}, ctx.Err()
				}
			}
			done := make(chan error, 1)
			go func() { done <- s.SampleHost(t.Context(), host, now) }()
			<-entered
			require.NoError(t, s.SampleHost(t.Context(), host, now.Add(time.Minute)))
			require.EqualValues(t, 1, s.SkippedSamples())
			switch change {
			case "disable":
				require.NoError(t, s.db.Model(&environment.Environment{}).Where("id=?", host.ID).Update("enabled", false).Error)
			case "revision":
				require.NoError(t, s.db.Model(&environment.Environment{}).Where("id=?", host.ID).Update("credential_revision", 1).Error)
			case "endpoint":
				require.NoError(t, s.db.Model(&environment.Environment{}).Where("id=?", host.ID).Update("api_url", "https://other.example.com").Error)
			case "remove":
				require.NoError(t, s.db.Delete(&host).Error)
			}
			close(release)
			require.NoError(t, <-done)
			var count int64
			require.NoError(t, s.db.Model(&bucketRow{}).Count(&count).Error)
			require.Zero(t, count)
			require.Empty(t, s.states)
			require.Empty(t, s.snapshots.read(host.ID).Items)
		})
	}
	s, host, now := samplerFixture(t)
	joined := make(chan struct{})
	s.budget = time.Millisecond
	s.fetch = func(ctx context.Context, _ environment.Environment) (dto.Sample, error) {
		<-ctx.Done()
		close(joined)
		return dto.Sample{}, ctx.Err()
	}
	require.NoError(t, s.SampleHost(t.Context(), host, now))
	select {
	case <-joined:
	default:
		t.Fatal("fetch did not honor cancellation")
	}
	var row bucketRow
	require.NoError(t, s.db.Where("kind='host' AND tier=15").Take(&row).Error)
	require.Equal(t, 1, row.MinutesDown)
}
func TestHB13ForgetClearsAllEphemeralState(t *testing.T) {
	s, host, now := samplerFixture(t)
	require.NoError(t, s.commit(t.Context(), host, now, fixtureSample(), nil))
	require.NotEmpty(t, s.states)
	s.Forget(host.ID)
	require.Empty(t, s.states)
	require.Empty(t, s.snapshots.read(host.ID).Items)
}
func TestHB15UptimeAndState(t *testing.T) {
	m := dto.Minutes{Running: 2, Unhealthy: 2, Down: 1, Stopped: 100}
	require.Equal(t, 60., *Uptime(m))
	require.Equal(t, dto.Down, BucketState(m))
	require.Nil(t, Uptime(dto.Minutes{Stopped: 10}))
	require.Equal(t, dto.Stopped, BucketState(dto.Minutes{Stopped: 10}))
	require.Equal(t, dto.NoData, BucketState(dto.Minutes{}))
}
func TestHB16IndependentLoadEpisodes(t *testing.T) {
	e := episode{}
	alerts := 0
	for minute := int64(60); minute <= 12*60; minute += 60 {
		var alert bool
		e, alert = evaluateLoad(e, minute, new(99.), 90, 5)
		if alert {
			alerts++
		}
	}
	require.Equal(t, 1, alerts)
	for minute := int64(13 * 60); minute <= 17*60; minute += 60 {
		e, _ = evaluateLoad(e, minute, new(20.), 90, 5)
	}
	for minute := int64(18 * 60); minute <= 22*60; minute += 60 {
		var alert bool
		e, alert = evaluateLoad(e, minute, new(99.), 90, 5)
		if alert {
			alerts++
		}
	}
	require.Equal(t, 2, alerts)
	e = episode{}
	for minute := int64(60); minute <= 4*60; minute += 60 {
		e, _ = evaluateLoad(e, minute, new(99.), 90, 5)
	}
	e, _ = evaluateLoad(e, 6*60, new(99.), 90, 5)
	require.Equal(t, 1, e.high)
	cpu, memory := episode{}, episode{}
	for minute := int64(60); minute <= 5*60; minute += 60 {
		var cpuAlert, memoryAlert bool
		cpu, cpuAlert = evaluateLoad(cpu, minute, new(99.), 90, 5)
		memory, memoryAlert = evaluateLoad(memory, minute, new(10.), 90, 5)
		require.False(t, memoryAlert)
		require.Equal(t, minute == 5*60, cpuAlert)
	}
}
func TestHB18CapacitySlopeAndSource(t *testing.T) {
	for _, slope := range []float64{10, 0, -10} {
		rows := []bucketRow{{BucketStart: 0, DiskSource: "old", DiskUsedLast: 10000, MinutesRunning: 1}, {BucketStart: 86400, DiskSource: "current", DiskUsedLast: 40, MinutesRunning: 1}, {BucketStart: 2 * 86400, DiskSource: "current", DiskUsedLast: 40 + slope, MinutesRunning: 1}, {BucketStart: 3 * 86400, DiskSource: "current", DiskUsedLast: 40 + 2*slope, MinutesRunning: 1}}
		current := rows[3]
		current.DiskTotalLast = 100
		c := capacityGrowth(rows, current)
		require.False(t, c.GrowthUnavailable)
		require.Equal(t, slope, *c.GrowthBytesPerDay)
		if slope > 0 {
			require.Equal(t, 4., *c.DaysUntilFull)
		} else {
			require.Nil(t, c.DaysUntilFull)
		}
		require.True(t, capacityGrowth(rows[3:], current).GrowthUnavailable)
	}
}

// N4: a host that is down at the start of the current two-hour bucket keeps its last measured usage.
func TestCapacityKeepsHostWithoutCurrentMeasurement(t *testing.T) {
	s, host, now := samplerFixture(t)
	base := now.Truncate(2 * time.Hour)
	for i, running := range []int{120, 120, 0} {
		start := base.Add(time.Duration(i-2) * 2 * time.Hour).Unix()
		require.NoError(t, s.db.Create(&bucketRow{Kind: "host", HostID: host.ID, EntityKey: host.ID, Tier: 120, BucketStart: start, LastMinute: start, MinutesRunning: running, MinutesDown: 120 - running, DiskUsedLast: float64(40 + i), DiskTotalLast: 100, DiskSource: map[bool]string{true: "/example", false: ""}[running > 0]}).Error)
	}
	got, err := s.Capacity(t.Context(), now)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, 41., got[0].UsedBytes)
}
