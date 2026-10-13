// Source: getarcaneapp/arcane/backend/internal/environment/children/swarm/helpers.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package swarm

import (
	"strings"
)

func buildAgentURLInternal(nodeID string) string {
	shortNodeID := nodeID
	if len(shortNodeID) > 12 {
		shortNodeID = shortNodeID[:12]
	}
	return "edge://swarm-node-" + shortNodeID
}

func buildAgentNameInternal(hostname, nodeID string) string {
	trimmedHostname := strings.TrimSpace(hostname)
	if trimmedHostname != "" {
		return trimmedHostname
	}
	if len(nodeID) > 12 {
		nodeID = nodeID[:12]
	}
	return "Swarm Node " + nodeID
}
