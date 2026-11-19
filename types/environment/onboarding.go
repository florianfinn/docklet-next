// Source: docklet-next/docs/design/host-onboarding.md
// Revision: 81439fab6a8ec5232161dc933fbd41bec41885fa
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original hub discovery and installation metadata; M29 container detection and classified address candidates.

package environment

import "time"

type HubAddressCandidate struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Reach  string `json:"reach" enum:"unknown,same-host"`
	Source string `json:"source" enum:"configured,interface,published,request"`
	Kind   string `json:"kind" enum:"ip,domain"`
}

type HubContainerNetwork struct {
	Name   string `json:"name"`
	Driver string `json:"driver"`
}

type HubPublishedPort struct {
	HostIP   string `json:"hostIp"`
	HostPort int    `json:"hostPort"`
}

type HubContainer struct {
	Name           string                `json:"name"`
	ListenPort     int                   `json:"listenPort"`
	Networks       []HubContainerNetwork `json:"networks"`
	PublishedPorts []HubPublishedPort    `json:"publishedPorts"`
}

// Interface addresses are hints, not a reachability check from the agent.
type HubDiscovery struct {
	ContainerDetection string                `json:"containerDetection" enum:"container,unknown"`
	Container          *HubContainer         `json:"container,omitempty"`
	DetectedAt         time.Time             `json:"detectedAt"`
	AgentImage         string                `json:"agentImage"`
	Candidates         []HubAddressCandidate `json:"candidates"`
}
