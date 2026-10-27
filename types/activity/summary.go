// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 86876feed1c0f8ad66d9a3eb54da6c4be5c3ebbc
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original list-only discovery, bounded reads and demand regression coverage (#225, #223).

package activity

// ActivitySummary is the optional HTTP-list projection. Runtime wire carriers
// and stream fingerprints continue to use Activity.
type ActivitySummary struct {
	Activity
	StepTrace []StepTraceEntry         `json:"stepTrace,omitzero"`
	Recovery  *ActivitySummaryRecovery `json:"recovery,omitzero"`
}

type ActivitySummaryRecovery struct {
	Disposition   *string `json:"disposition,omitzero"`
	UnknownReason *string `json:"unknownReason,omitzero"`
}
