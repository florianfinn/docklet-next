// Source: docklet-next/docs/design/images-updates.md
// Revision: 727498b22e03475aa5b4e5325727ff182e8cebdd
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: M6 retention precedence under failed states and active/unknown holds.

package lifecycle

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRetentionPreservesLatestRestoreTarget(t *testing.T) {
	points := retentionPoints(7, "settled")
	used := time.Unix(100, 0)
	points[6].LastRestoredAt = &used
	deleted := RetentionDeletions(points, nil)
	require.Len(t, deleted, 2)
	require.NotContains(t, deleted, points[6].ID, "the oldest restore target is still protected")
	require.NotContains(t, deleted, points[0].ID, "latest healthy baseline is protected")
	require.Equal(t, []string{points[5].ID, points[4].ID}, deleted, "oldest other points go first")
}
func TestRetentionProtectsBaselineBeyondHeldLimit(t *testing.T) {
	points := retentionPoints(7, "partial-source")
	held := map[string]bool{}
	for _, p := range points {
		held[p.ID] = true
	}
	points = append(points, Point{ID: "healthy-baseline", SourceState: "settled"})
	require.Empty(t, RetentionDeletions(points, held), "active/unknown holds cannot evict the last healthy baseline")
}
func TestRetentionKeepsOneFailedState(t *testing.T) {
	points := retentionPoints(5, "partial-source")
	points = append(points, Point{ID: "baseline", SourceState: "settled"})
	deleted := RetentionDeletions(points, nil)
	require.Len(t, deleted, 4, "replace failed snapshots even below the five-point limit")
	require.NotContains(t, deleted, points[0].ID)
	require.NotContains(t, deleted, "baseline")
}
func TestRetentionFailedRestoreTargetOwnsFailedSlot(t *testing.T) {
	points := retentionPoints(4, "partial-source")
	used := time.Unix(100, 0)
	points[3].LastRestoredAt = &used
	points = append(points, Point{ID: "baseline", SourceState: "settled"})
	deleted := RetentionDeletions(points, nil)
	require.ElementsMatch(t, []string{points[0].ID, points[1].ID, points[2].ID}, deleted)
}
func TestRetentionFailedHoldsOutliveReplacement(t *testing.T) {
	points := retentionPoints(3, "partial-source")
	held := map[string]bool{points[2].ID: true}
	require.Equal(t, []string{points[1].ID}, RetentionDeletions(points, held), "the failed slot does not release an active or unknown owner")
}
func retentionPoints(n int, state string) []Point {
	points := make([]Point, n)
	for i := range points {
		points[i] = Point{ID: fmt.Sprintf("point-%d", i), SourceState: state, FailedAttempt: state == "partial-source", CreatedAt: time.Unix(int64(n-i), 0)}
	}
	return points
}
