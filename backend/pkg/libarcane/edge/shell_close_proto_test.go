// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original shell close metadata over the existing protobuf envelope.

package edge

import (
	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestShellCloseMetadataProtoRoundTrip(t *testing.T) {
	for _, code := range []websocket.StatusCode{websocket.StatusNormalClosure, websocket.StatusPolicyViolation} {
		reason := "container.shell-protocol"
		encoded := encodeStreamClose(websocket.CloseError{Code: code, Reason: reason})
		message, err := tunnelMessageToAgentProto(&TunnelMessage{ID: "example", Type: MessageTypeStreamClose, Error: encoded})
		require.NoError(t, err)
		roundTrip, err := agentProtoToTunnelMessage(message)
		require.NoError(t, err)
		status, text := decodeStreamClose(roundTrip.Error)
		require.Equal(t, code, status)
		require.Equal(t, reason, text)
	}
	status, reason := decodeStreamClose("")
	require.Equal(t, websocket.StatusNormalClosure, status)
	require.Empty(t, reason)
	status, reason = decodeStreamClose("legacy error")
	require.Equal(t, websocket.StatusInternalError, status)
	require.Equal(t, "legacy error", reason)
}
