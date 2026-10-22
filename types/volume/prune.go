// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 60d2471b62b12e1af64c54060a50102a2e39d1c1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original stateless prune contract implementation for issue #179.
package volume

// PruneCandidate binds a submitted removal to the observed Docker identity.
type PruneCandidate struct {
	Name      string `json:"name" minLength:"1"`
	CreatedAt string `json:"createdAt" minLength:"1"`
}

type PrunePlanCandidate struct {
	Name      string `json:"name"`
	Driver    string `json:"driver"`
	CreatedAt string `json:"createdAt"`
	Reason    string `json:"reason" enum:"unused,unused-anonymous"`
	SizeBytes *int64 `json:"sizeBytes,omitempty" minimum:"0"`
}

type PrunePlan struct {
	Candidates []PrunePlanCandidate `json:"candidates"`
}

type PruneRequest struct {
	Candidates []PruneCandidate `json:"candidates" maxItems:"10000"`
}

type PruneSkipped struct {
	Name   string `json:"name"`
	Reason string `json:"reason" enum:"in-use,not-found,changed"`
}

type PruneFailed struct {
	Name string `json:"name"`
	Code string `json:"code" enum:"volume.inspect-failed,volume.usage-failed,volume.remove-failed,volume.protected"`
}

// PruneResult reports each candidate independently. SpaceReclaimed is a lower
// bound when Docker could not provide sizes for successful removals.
type PruneResult struct {
	Removed        []string       `json:"removed"`
	Skipped        []PruneSkipped `json:"skipped"`
	Failed         []PruneFailed  `json:"failed"`
	SpaceReclaimed uint64         `json:"spaceReclaimed"`
	ActivityID     *string        `json:"activityId,omitempty"`
}
