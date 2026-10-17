// Source: docklet-next/docs/design/host-onboarding.md
// Revision: 81439fab6a8ec5232161dc933fbd41bec41885fa
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original hub discovery and installation metadata.

package environment

import "time"

type HubAddressCandidate struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Reach  string `json:"reach" enum:"unknown,same-host"`
	Source string `json:"source" enum:"configured,interface"`
}

// Interface addresses are hints, not a reachability check from the agent.
type HubDiscovery struct {
	DetectedAt time.Time             `json:"detectedAt"`
	AgentImage string                `json:"agentImage"`
	Candidates []HubAddressCandidate `json:"candidates"`
}
