// Source: docklet-next/docs/design/images-updates.md
// Revision: 727498b22e03475aa5b4e5325727ff182e8cebdd
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: M6 bounded retention preserves restore targets and healthy baseline evidence.

package lifecycle

import dto "github.com/florianfinn/docklet-next/types/updater"

// RetentionDeletions receives newest-first available points. Holds take
// precedence over limits; a failed-attempt restore target owns the failed slot.
func RetentionDeletions(points []Point, held map[string]bool) []string {
	return PlanRetention(points, held).Deletions
}

type RetentionPlan struct {
	Deletions []string
	Reasons   map[string]dto.RestorePointProtectionReason
}

func PlanRetention(points []Point, held map[string]bool) RetentionPlan {
	reasons := map[string]dto.RestorePointProtectionReason{}
	protected := map[string]bool{}
	for id, active := range held {
		if active {
			protected[id] = true
		}
	}
	var baseline, restored *Point
	for i := range points {
		p := &points[i]
		if baseline == nil && p.SourceState == "settled" {
			baseline = p
			reasons[p.ID] = dto.HealthyBaseline
			protected[p.ID] = true
		}
		if p.LastRestoredAt != nil && (restored == nil || p.LastRestoredAt.After(*restored.LastRestoredAt)) {
			restored = p
		}
	}
	if restored != nil {
		protected[restored.ID] = true
		reasons[restored.ID] = dto.RestoreTarget
	}
	failed := ""
	if restored != nil && restored.FailedAttempt && restored.SourceState == "partial-source" {
		failed = restored.ID
	}
	var deleted []string
	var retained []Point
	for _, p := range points {
		if p.FailedAttempt && p.SourceState == "partial-source" && !held[p.ID] {
			if failed == "" {
				failed = p.ID
			}
			if p.ID != failed && !protected[p.ID] {
				deleted = append(deleted, p.ID)
				continue
			}
		}
		retained = append(retained, p)
	}
	for i := len(retained) - 1; len(retained) > 5 && i >= 0; i-- {
		if protected[retained[i].ID] {
			continue
		}
		deleted = append(deleted, retained[i].ID)
		retained = append(retained[:i], retained[i+1:]...)
	}
	return RetentionPlan{Deletions: deleted, Reasons: reasons}
}
