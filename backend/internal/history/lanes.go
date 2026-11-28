// Source: docklet-next/issues/693
// Revision: 1829fa7776d511dab9604d9b5e3a81cfc7c55640
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original host container lanes for the Disruptions page.

package history

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	dto "github.com/florianfinn/docklet-next/types/history"
)

// MaxLanes bounds one host's lanes; the most disrupted containers are kept.
const MaxLanes = 500

// readHostContainers returns, from one read of the range's tier, the aligned bucket series of
// every container name on the host that had a down or unhealthy minute in the range.
func readHostContainers(ctx context.Context, db *database.DB, host, value string, now time.Time) (dto.HostContainers, error) {
	out := dto.HostContainers{Range: value, HostID: host, Containers: []dto.ContainerLane{}}
	tier, count, err := rangeShape(value)
	if err != nil {
		return out, err
	}
	step := int64(tier * 60)
	end := now.Unix() / step * step
	start := end - int64(count-1)*step
	rows := []bucketRow{}
	if err = db.WithContext(ctx).Select("entity_key", "bucket_start", "minutes_running", "minutes_unhealthy", "minutes_down", "minutes_stopped", "exit_code_last").
		Where("kind='container' AND host_id=? AND tier=? AND bucket_start BETWEEN ? AND ?", host, tier, start, end).Find(&rows).Error; err != nil {
		return out, err
	}
	byName := map[string]map[int64]bucketRow{}
	disrupted := map[string]int{}
	for _, r := range rows {
		if byName[r.EntityKey] == nil {
			byName[r.EntityKey] = map[int64]bucketRow{}
		}
		byName[r.EntityKey][r.BucketStart] = r
		disrupted[r.EntityKey] += r.MinutesDown + r.MinutesUnhealthy
	}
	names := []string{}
	for name, minutes := range disrupted {
		if minutes > 0 {
			names = append(names, name)
		}
	}
	slices.SortFunc(names, func(a, b string) int { return cmp.Or(disrupted[b]-disrupted[a], cmp.Compare(a, b)) })
	if len(names) > MaxLanes {
		names, out.Truncated = names[:MaxLanes], true
	}
	slices.Sort(names)
	for _, name := range names {
		lane := dto.ContainerLane{Name: name, Buckets: make([]dto.LaneBucket, 0, count)}
		for i := range count {
			stamp := start + int64(i)*step
			r := byName[name][stamp]
			b := dto.LaneBucket{Start: time.Unix(stamp, 0).UTC(), Minutes: dto.Minutes{Running: r.MinutesRunning, Unhealthy: r.MinutesUnhealthy, Down: r.MinutesDown, Stopped: r.MinutesStopped}, ExitCode: r.ExitCodeLast}
			b.State = BucketState(b.Minutes)
			lane.Buckets = append(lane.Buckets, b)
		}
		out.Containers = append(out.Containers, lane)
	}
	return out, nil
}
