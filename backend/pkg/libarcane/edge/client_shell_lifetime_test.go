// Source: getarcaneapp/arcane/backend/pkg/libarcane/edge/client_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Extracted gRPC method constant regression; Local module namespace; relocated WebSocket origin/forwarding test to client_websocket_origin_test.go.

package edge

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTunnelClient_GRPCConnectMethodInternal(t *testing.T) {
	client := NewTunnelClient(&Config{}, http.NotFoundHandler())
	assert.Equal(t, "/api/tunnel/connect", client.grpcConnectMethodInternal())
}
