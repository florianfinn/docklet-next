// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Producer and transport regression proofs for #229 point projection.

package lifecycle

import (
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRetentionProjectionAnchorsAndFailedSlot(t *testing.T) {
	points := retentionPoints(7, "settled")
	used := time.Unix(100, 0)
	points[6].LastRestoredAt = &used
	plan := PlanRetention(points, nil)
	require.Equal(t, dto.HealthyBaseline, plan.Reasons[points[0].ID])
	require.Equal(t, dto.RestoreTarget, plan.Reasons[points[6].ID])
	require.Equal(t, []string{points[5].ID, points[4].ID}, plan.Deletions)
	points[6].LastRestoredAt = nil
	points[6].SourceState = "partial-source"
	points[6].FailedAttempt = true
	plan = PlanRetention(points, nil)
	require.NotContains(t, plan.Reasons, points[6].ID)
	require.Contains(t, plan.Deletions, points[6].ID)
	held := map[string]bool{}
	for _, p := range points {
		held[p.ID] = true
	}
	require.Empty(t, PlanRetention(points, held).Deletions)
}

func TestRetentionProjectionOverlappingAnchors(t *testing.T) {
	points := retentionPoints(1, "settled")
	used := time.Unix(100, 0)
	points[0].LastRestoredAt = &used
	plan := PlanRetention(points, nil)
	require.Equal(t, dto.RestoreTarget, plan.Reasons[points[0].ID])
	require.Empty(t, plan.Deletions)
}
