// Source: docklet-next/docs/design/images-updates.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Informational agent/tools references with a hub-computed stable release ceiling.

package updater

type SystemImageHint struct {
	Kind                string `json:"kind" enum:"agent,tools"`
	Reference           string `json:"reference"`
	CurrentVersion      string `json:"currentVersion,omitempty"`
	CandidateVersion    string `json:"candidateVersion,omitempty"`
	CompatibleReference string `json:"compatibleReference,omitempty"`
	State               string `json:"state" enum:"current,available,pinned,unknown"`
	Code                string `json:"code,omitempty"`
	RequiresHubUpdate   bool   `json:"requiresHubUpdate"`
	ManualOnly          bool   `json:"manualOnly"`
}
type SystemImageHints struct {
	HubVersion string            `json:"hubVersion"`
	Images     []SystemImageHint `json:"images"`
}
