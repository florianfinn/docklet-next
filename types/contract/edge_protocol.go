// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Edge protocol marker; no refusal path while the minimum is 1.

package contract

import (
	"strconv"
	"strings"
)

// EdgeProtocolVersion is the tunnel framing version this build speaks. It is a
// transport capability, not part of the agent contract fingerprint.
const EdgeProtocolVersion = 1

// MinimumEdgeProtocolVersion is the oldest peer protocol this build accepts.
// The release that raises it also ships the refusal path and its tests.
const MinimumEdgeProtocolVersion = 1

const edgeProtocolPrefix = "edge-protocol/"

// EdgeProtocol is the peer protocol parsed from register capabilities.
type EdgeProtocol struct {
	Version int
	// Advertised is false when the peer sent no usable marker (counts as 1).
	Advertised bool
	// Malformed reports duplicate, conflicting or unparsable markers.
	Malformed bool
}

// EdgeProtocolCapability is this build's own marker.
func EdgeProtocolCapability() string {
	return edgeProtocolPrefix + strconv.Itoa(EdgeProtocolVersion)
}

// IsEdgeProtocolCapability reports whether a capability is any protocol marker.
func IsEdgeProtocolCapability(capability string) bool {
	return strings.HasPrefix(capability, edgeProtocolPrefix)
}

// ParseEdgeProtocol reads the peer marker. A missing, malformed, duplicate or
// conflicting marker counts as protocol 1 so pre-marker peers stay compatible.
func ParseEdgeProtocol(capabilities []string) EdgeProtocol {
	markers := 0
	version := 0
	malformed := false
	for _, capability := range capabilities {
		if !IsEdgeProtocolCapability(capability) {
			continue
		}
		markers++
		raw := strings.TrimPrefix(capability, edgeProtocolPrefix)
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || strconv.Itoa(parsed) != raw {
			malformed = true
			continue
		}
		version = parsed
	}
	if markers == 0 {
		return EdgeProtocol{Version: 1}
	}
	if malformed || markers > 1 {
		return EdgeProtocol{Version: 1, Malformed: true}
	}
	return EdgeProtocol{Version: version, Advertised: true}
}

// WithoutEdgeProtocol drops every protocol marker so a side never echoes the
// peer's value as its own.
func WithoutEdgeProtocol(capabilities []string) []string {
	out := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		if !IsEdgeProtocolCapability(capability) {
			out = append(out, capability)
		}
	}
	return out
}
