// Source: docklet-next/issues/337
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original least-squares capacity trend over the current disk source.

package history

import dto "github.com/florianfinn/docklet-next/types/history"

func capacityGrowth(rows []bucketRow, current bucketRow) dto.Capacity {
	out := dto.Capacity{HostID: current.HostID, DiskSource: current.DiskSource, UsedBytes: current.DiskUsedLast, TotalBytes: current.DiskTotalLast, GrowthUnavailable: true}
	points := []bucketRow{}
	for _, r := range rows {
		if r.DiskSource == current.DiskSource && r.MinutesRunning > 0 {
			points = append(points, r)
		}
	}
	if len(points) < 2 || points[len(points)-1].BucketStart-points[0].BucketStart < 24*3600 {
		return out
	}
	var sx, sy, sxx, sxy float64
	for _, r := range points {
		x := float64(r.BucketStart-points[0].BucketStart) / 86400
		y := r.DiskUsedLast
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	n := float64(len(points))
	denominator := n*sxx - sx*sx
	if denominator <= 0 {
		return out
	}
	slope := (n*sxy - sx*sy) / denominator
	out.GrowthUnavailable = false
	out.GrowthBytesPerDay = new(slope)
	if slope > 0 {
		out.DaysUntilFull = new(max(0, current.DiskTotalLast-current.DiskUsedLast) / slope)
	}
	return out
}
