// Source: docklet-next/issues/347
// Revision: 6827ad2256d11a1e63e395c03dbb89d8ba7d6fde
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original response-burst delivery proof for the hub tunnel receive loop.

package edge

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// More response chunks than the pending buffer holds, arriving before the
// consumer reads, must all reach it instead of failing the request.
func TestResponseBurstWaitsForTheConsumer(t *testing.T) {
	tunnel := &AgentTunnel{EnvironmentID: "example"}
	pending, err := registerPendingRequestInternal(tunnel, "burst")
	require.NoError(t, err)
	chunks := cap(pending.ResponseCh) + 64
	timer := time.NewTimer(streamDeliveryTimeout)
	timer.Stop()
	delivered := make(chan struct{})
	go func() {
		defer close(delivered)
		server := &TunnelServer{}
		for range chunks {
			server.deliverResponse(t.Context(), tunnel, &TunnelMessage{ID: "burst", Type: MessageTypeCommandOutput, Body: []byte("x")}, timer)
		}
	}()
	time.Sleep(100 * time.Millisecond)
	for range chunks {
		select {
		case <-pending.ResponseCh:
		case failure := <-pending.failureCh:
			t.Fatalf("burst failed the request: %v", failure)
		case <-time.After(2 * time.Second):
			t.Fatal("chunk not delivered")
		}
	}
	<-delivered
	_, still := tunnel.Pending.Load("burst")
	require.True(t, still, "the request stays pending")
}
