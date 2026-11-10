// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original agent-side Edge protocol record; kept apart from the size-capped client.go.

package edge

import (
	"context"
	"log/slog"

	"github.com/florianfinn/docklet-next/types/contract"
)

// HubEdgeProtocol returns the manager's protocol from the last accepted
// registration, or 0 before the first registration.
func (c *TunnelClient) HubEdgeProtocol() int {
	if c == nil {
		return 0
	}
	registration, _ := c.registration.Load()
	return registration.hubEdgeProtocol
}

// hubEdgeProtocolInternal parses the manager's marker from an accepted
// registration reply and logs it once per registration.
func hubEdgeProtocolInternal(ctx context.Context, reply *TunnelMessage) int {
	protocol := contract.ParseEdgeProtocol(reply.Capabilities)
	if protocol.Malformed {
		slog.WarnContext(ctx, "Edge manager sent a malformed protocol marker; treating it as protocol 1", "environmentId", reply.EnvironmentID)
	}
	slog.InfoContext(ctx, "Edge manager protocol", "environmentId", reply.EnvironmentID, "hubEdgeProtocol", protocol.Version)
	return protocol.Version
}

// ownEdgeProtocolCapabilityInternal is the marker the agent advertises.
func ownEdgeProtocolCapabilityInternal() string {
	return contract.EdgeProtocolCapability()
}
