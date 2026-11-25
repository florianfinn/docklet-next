// Source: docklet-next/issues/626
// Revision: 6dc802124ce20a9c53693493ca66a85d1820ef19
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original M41 derived counts of planned update-check targets.

package scheduler

// BulkCounts derives the counts of a multi-target outcome: only targets in the
// current plan count, so retained targets of an earlier attempt never inflate them.
type BulkCounts struct {
	Planned, Completed, Succeeded, Failed, Skipped, Updates int
	PlannedKnown                                            bool
}

// Counts returns false when the outcome has no plan (legacy runs and single-target jobs).
func (o Outcome) Counts() (BulkCounts, bool) {
	if o.PlannedIDs == nil && !o.PlannedKnown {
		return BulkCounts{}, false
	}
	planned := make(map[string]bool, len(o.PlannedIDs))
	for _, id := range o.PlannedIDs {
		planned[id] = true
	}
	counts := BulkCounts{Planned: len(planned), PlannedKnown: o.PlannedKnown}
	for _, target := range o.Targets {
		if !planned[target.ID] {
			continue
		}
		switch target.Status {
		case Succeeded:
			counts.Succeeded++
		case Failed:
			counts.Failed++
		case Skipped:
			counts.Skipped++
		default:
			continue
		}
		counts.Completed++
		if target.UpdateAvailable {
			counts.Updates++
		}
	}
	return counts, true
}
