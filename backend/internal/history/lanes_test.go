// Source: docklet-next/issues/693
// Revision: 1829fa7776d511dab9604d9b5e3a81cfc7c55640
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original host container lane break-map tests.

package history

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	dto "github.com/florianfinn/docklet-next/types/history"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Rows written by the sampler, including rows that existed before this route, are read as stored.
func TestHostContainerLanesSelectDisruptedContainersOnly(t *testing.T) {
	db := historyDB(t)
	now := time.Date(2026, 11, 3, 12, 7, 0, 0, time.UTC)
	minute := now.Truncate(time.Minute).Unix()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		for _, o := range []observation{
			{kind: "container", key: "web", state: dto.Down, exitCode: 3},
			{kind: "container", key: "api", state: dto.Unhealthy},
			{kind: "container", key: "quiet", state: dto.Running},
			{kind: "container", key: "parked", state: dto.Stopped},
			{kind: "stack", key: "web", state: dto.Down},
		} {
			if err := upsertMinute(tx, "host", minute, o); err != nil {
				return err
			}
		}
		return upsertMinute(tx, "other", minute, observation{kind: "container", key: "elsewhere", state: dto.Down})
	}))
	// A row from before this route, written directly, in an older bucket of the same range.
	older := now.Add(-3*time.Hour).Unix() / 900 * 900
	require.NoError(t, db.Create(&bucketRow{Kind: "container", HostID: "host", EntityKey: "quiet", Tier: 15, BucketStart: older, LastMinute: older, MinutesUnhealthy: 2, MinutesRunning: 13}).Error)

	got, err := readHostContainers(t.Context(), db, "host", "24h", now)
	require.NoError(t, err)
	require.Equal(t, "host", got.HostID)
	require.False(t, got.Truncated)
	names := []string{}
	for _, lane := range got.Containers {
		names = append(names, lane.Name)
		require.Len(t, lane.Buckets, 96)
		require.Equal(t, dto.NoData, lane.Buckets[0].State)
	}
	require.Equal(t, []string{"api", "quiet", "web"}, names)
	web := got.Containers[2].Buckets[95]
	require.Equal(t, dto.Down, web.State)
	require.Equal(t, 1, web.Minutes.Down)
	require.Equal(t, 3, web.ExitCode)
	require.Equal(t, now.Unix()/900*900, web.Start.Unix())
	quiet := got.Containers[1]
	require.Equal(t, dto.Running, quiet.Buckets[95].State)
	require.Equal(t, dto.Unhealthy, quiet.Buckets[95-12].State)

	empty, err := readHostContainers(t.Context(), db, "unknown", "7d", now)
	require.NoError(t, err)
	require.Empty(t, empty.Containers)
	require.Equal(t, "7d", empty.Range)

	_, err = readHostContainers(t.Context(), db, "host", "1h", now)
	var problem *dto.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, "history.invalid-range", problem.Code)
}

// The cap keeps the most disrupted containers, ties by name, and marks the cut.
func TestHostContainerLanesCapKeepsMostDisrupted(t *testing.T) {
	db := historyDB(t)
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	start := now.Unix() / 900 * 900
	rows := []bucketRow{}
	for i := range MaxLanes + 2 {
		down := 1
		if i == MaxLanes+1 {
			down = 5
		}
		rows = append(rows, bucketRow{Kind: "container", HostID: "host", EntityKey: fmt.Sprintf("c%04d", i), Tier: 15, BucketStart: start, LastMinute: start, MinutesDown: down})
	}
	require.NoError(t, db.CreateInBatches(rows, 100).Error)
	got, err := readHostContainers(t.Context(), db, "host", "24h", now)
	require.NoError(t, err)
	require.True(t, got.Truncated)
	require.Len(t, got.Containers, MaxLanes)
	require.Equal(t, "c0000", got.Containers[0].Name)
	require.Equal(t, fmt.Sprintf("c%04d", MaxLanes+1), got.Containers[MaxLanes-1].Name, "the most disrupted container survives the cap")
	for _, lane := range got.Containers {
		require.NotEqual(t, fmt.Sprintf("c%04d", MaxLanes), lane.Name, "the last tie by name is cut")
	}
}

// Buckets stored before a hub restart, without a later sample, are read from the reopened database.
func TestHostContainerLanesReadPersistedBucketsAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	now := time.Date(2026, 11, 3, 12, 7, 0, 0, time.UTC)
	older := now.Add(-5*time.Hour).Unix() / 900 * 900
	db, err := database.Initialize(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
	require.NoError(t, err)
	require.NoError(t, db.Create(&bucketRow{Kind: "container", HostID: "host", EntityKey: "restarted", Tier: 15, BucketStart: older, LastMinute: older + 840, MinutesDown: 4, MinutesRunning: 11, ExitCodeLast: 137}).Error)
	require.NoError(t, db.Close())

	reopened, err := database.Initialize(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	got, err := readHostContainers(t.Context(), reopened, "host", "24h", now)
	require.NoError(t, err)
	require.Len(t, got.Containers, 1)
	lane := got.Containers[0]
	require.Equal(t, "restarted", lane.Name)
	require.Len(t, lane.Buckets, 96)
	at := 95 - 20
	require.Equal(t, older, lane.Buckets[at].Start.Unix(), "the stored bucket keeps its aligned slot")
	require.Equal(t, dto.Down, lane.Buckets[at].State)
	require.Equal(t, 4, lane.Buckets[at].Minutes.Down)
	require.Equal(t, 137, lane.Buckets[at].ExitCode)
	require.Equal(t, dto.NoData, lane.Buckets[95].State, "no sample after the restart leaves the open bucket empty")
}
