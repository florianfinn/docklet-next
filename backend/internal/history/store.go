// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original three-tier minute-guarded history storage and fixed bucket reads.

package history

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dto "github.com/florianfinn/docklet-next/types/history"
	"gorm.io/gorm"
	"strings"
	"time"
)

type bucketRow struct {
	Kind             string `gorm:"primaryKey"`
	HostID           string `gorm:"primaryKey;index:idx_history_host"`
	EntityKey        string `gorm:"primaryKey"`
	Tier             int    `gorm:"primaryKey;autoIncrement:false"`
	BucketStart      int64  `gorm:"primaryKey;autoIncrement:false"`
	LastMinute       int64
	MinutesRunning   int
	MinutesUnhealthy int
	MinutesDown      int
	MinutesStopped   int
	CPUSum           float64
	CPUN             int
	CPUMax           float64
	MemSum           float64
	MemN             int
	MemMax           float64
	MemLimitMax      float64
	NetRx            float64
	NetTx            float64
	BlkRead          float64
	BlkWrite         float64
	DiskUsedLast     float64
	DiskTotalLast    float64
	DiskSource       string
	ExitCodeLast     int
}

func (bucketRow) TableName() string { return "history_buckets" }

// Models supplies the history-owned shape to the development schema proof.
func Models() []any { return []any{&bucketRow{}} }

type observation struct {
	kind, key, identity                                   string
	state                                                 dto.State
	cpu, memory                                           *float64
	memoryLimit, rx, tx, read, write, diskUsed, diskTotal float64
	diskSource                                            string
	exitCode                                              int
}

func upsertMinute(tx *gorm.DB, host string, minute int64, o observation) error {
	for _, tier := range []int{15, 120, 480} {
		row := bucketRow{Kind: o.kind, HostID: host, EntityKey: o.key, Tier: tier, BucketStart: minute / int64(tier*60) * int64(tier*60), LastMinute: minute, MemLimitMax: o.memoryLimit, NetRx: o.rx, NetTx: o.tx, BlkRead: o.read, BlkWrite: o.write, DiskUsedLast: o.diskUsed, DiskTotalLast: o.diskTotal, DiskSource: o.diskSource, ExitCodeLast: o.exitCode}
		switch o.state {
		case dto.Running:
			row.MinutesRunning = 1
		case dto.Unhealthy:
			row.MinutesUnhealthy = 1
		case dto.Down:
			row.MinutesDown = 1
		case dto.Stopped:
			row.MinutesStopped = 1
		default:
			continue
		}
		if o.cpu != nil {
			row.CPUSum, row.CPUMax, row.CPUN = *o.cpu, *o.cpu, 1
		}
		if o.memory != nil {
			row.MemSum, row.MemMax, row.MemN = *o.memory, *o.memory, 1
		}
		columns := []string{"kind", "host_id", "entity_key", "tier", "bucket_start", "last_minute", "minutes_running", "minutes_unhealthy", "minutes_down", "minutes_stopped", "cpu_sum", "cpu_n", "cpu_max", "mem_sum", "mem_n", "mem_max", "mem_limit_max", "net_rx", "net_tx", "blk_read", "blk_write", "disk_used_last", "disk_total_last", "disk_source", "exit_code_last"}
		values := []any{row.Kind, row.HostID, row.EntityKey, row.Tier, row.BucketStart, row.LastMinute, row.MinutesRunning, row.MinutesUnhealthy, row.MinutesDown, row.MinutesStopped, row.CPUSum, row.CPUN, row.CPUMax, row.MemSum, row.MemN, row.MemMax, row.MemLimitMax, row.NetRx, row.NetTx, row.BlkRead, row.BlkWrite, row.DiskUsedLast, row.DiskTotalLast, row.DiskSource, row.ExitCodeLast}
		updates := []string{}
		for _, c := range []string{"minutes_running", "minutes_unhealthy", "minutes_down", "minutes_stopped", "cpu_sum", "cpu_n", "mem_sum", "mem_n", "net_rx", "net_tx", "blk_read", "blk_write"} {
			updates = append(updates, c+"=history_buckets."+c+"+excluded."+c)
		}
		for _, c := range []string{"cpu_max", "mem_max", "mem_limit_max"} {
			updates = append(updates, c+"=MAX(history_buckets."+c+",excluded."+c+")")
		}
		for _, c := range []string{"last_minute", "exit_code_last"} {
			updates = append(updates, c+"=excluded."+c)
		}
		for _, c := range []string{"disk_used_last", "disk_total_last", "disk_source"} {
			updates = append(updates, c+"=CASE WHEN excluded.minutes_running>0 THEN excluded."+c+" ELSE history_buckets."+c+" END")
		}
		sql := "INSERT INTO history_buckets (" + strings.Join(columns, ",") + ") VALUES (" + strings.TrimSuffix(strings.Repeat("?,", len(values)), ",") + ") ON CONFLICT(kind,host_id,entity_key,tier,bucket_start) DO UPDATE SET " + strings.Join(updates, ",") + " WHERE excluded.last_minute > history_buckets.last_minute"
		if err := tx.Exec(sql, values...).Error; err != nil {
			return err
		}
	}
	return nil
}

func rangeShape(value string) (tier, count int, err error) {
	switch value {
	case "24h":
		return 15, 96, nil
	case "7d":
		return 120, 84, nil
	case "30d":
		return 480, 90, nil
	}
	return 0, 0, dto.Fail("history.invalid-range", 400)
}
func readHistory(ctx context.Context, db *database.DB, kind, host, key, value string, now time.Time) (dto.Response, error) {
	out := dto.Response{Range: value, Buckets: []dto.Bucket{}}
	tier, count, err := rangeShape(value)
	if err != nil {
		return out, err
	}
	step := int64(tier * 60)
	end := now.Unix() / step * step
	start := end - int64(count-1)*step
	rows := []bucketRow{}
	if err = db.WithContext(ctx).Where("kind=? AND host_id=? AND entity_key=? AND tier=? AND bucket_start BETWEEN ? AND ?", kind, host, key, tier, start, end).Order("bucket_start").Find(&rows).Error; err != nil {
		return out, err
	}
	byStart := map[int64]bucketRow{}
	for _, r := range rows {
		byStart[r.BucketStart] = r
	}
	for i := 0; i < count; i++ {
		stamp := start + int64(i)*step
		r := byStart[stamp]
		b := dto.Bucket{Start: time.Unix(stamp, 0).UTC(), Partial: i == count-1, Minutes: dto.Minutes{Running: r.MinutesRunning, Unhealthy: r.MinutesUnhealthy, Down: r.MinutesDown, Stopped: r.MinutesStopped}, MemoryLimit: r.MemLimitMax, NetworkRx: r.NetRx, NetworkTx: r.NetTx, BlockRead: r.BlkRead, BlockWrite: r.BlkWrite, DiskUsed: r.DiskUsedLast, DiskTotal: r.DiskTotalLast, DiskSource: r.DiskSource, ExitCode: r.ExitCodeLast}
		b.State = BucketState(b.Minutes)
		if r.CPUN > 0 {
			b.CPUAverage = new(r.CPUSum / float64(r.CPUN))
			b.CPUMax = new(r.CPUMax)
		}
		if r.MemN > 0 {
			b.MemoryAverage = new(r.MemSum / float64(r.MemN))
			b.MemoryMax = new(r.MemMax)
		}
		out.Totals.Running += b.Minutes.Running
		out.Totals.Unhealthy += b.Minutes.Unhealthy
		out.Totals.Down += b.Minutes.Down
		out.Totals.Stopped += b.Minutes.Stopped
		out.Buckets = append(out.Buckets, b)
	}
	out.UptimePercent = Uptime(out.Totals)
	return out, nil
}
func BucketState(m dto.Minutes) dto.State {
	switch {
	case m.Down > 0:
		return dto.Down
	case m.Unhealthy > 0:
		return dto.Unhealthy
	case m.Running > 0:
		return dto.Running
	case m.Stopped > 0:
		return dto.Stopped
	default:
		return dto.NoData
	}
}
func Uptime(m dto.Minutes) *float64 {
	denominator := m.Running + m.Unhealthy + m.Down
	if denominator == 0 {
		return nil
	}
	return new(100 * (float64(m.Running) + 0.5*float64(m.Unhealthy)) / float64(denominator))
}
func retain(ctx context.Context, db *database.DB, now time.Time) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for tier, hours := range map[int]int{15: 24, 120: 7 * 24, 480: 30 * 24} {
			if err := tx.Where("tier=? AND bucket_start < ?", tier, now.Add(-time.Duration(hours)*time.Hour).Unix()).Delete(&bucketRow{}).Error; err != nil {
				return fmt.Errorf("prune history: %w", err)
			}
		}
		return nil
	})
}
