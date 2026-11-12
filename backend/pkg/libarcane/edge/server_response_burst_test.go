// Source: docklet-next/issues/347
// Revision: 6827ad2256d11a1e63e395c03dbb89d8ba7d6fde
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original response-burst delivery proof for the hub tunnel receive loop.

package edge

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func deliverBurstInternal(tunnel *AgentTunnel, chunks int) <-chan struct{} {
	timer := time.NewTimer(streamDeliveryTimeout)
	timer.Stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server := &TunnelServer{}
		for range chunks {
			server.deliverResponse(context.Background(), tunnel, &TunnelMessage{ID: "burst", Type: MessageTypeCommandOutput, Body: []byte("x")}, timer)
		}
	}()
	return done
}

// More response chunks than the pending buffer holds wait for the consumer
// instead of failing the request.
func TestResponseBurstWaitsForTheConsumer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tunnel := &AgentTunnel{EnvironmentID: "example"}
		pending, err := registerPendingRequestInternal(tunnel, "burst")
		require.NoError(t, err)
		chunks := cap(pending.ResponseCh) + 64
		done := deliverBurstInternal(tunnel, chunks)
		synctest.Wait()
		require.Len(t, pending.ResponseCh, cap(pending.ResponseCh), "the buffer is full and the next delivery waits")
		require.Empty(t, pending.failureCh, "a full buffer does not fail the request")
		for range chunks {
			<-pending.ResponseCh
		}
		<-done
		require.Empty(t, pending.failureCh)
		_, still := tunnel.Pending.Load("burst")
		require.True(t, still, "the request stays pending")
	})
}

// A consumer that stays stuck still fails its request after the delivery timeout.
func TestResponseBurstFailsAStuckConsumer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tunnel := &AgentTunnel{EnvironmentID: "example"}
		pending, err := registerPendingRequestInternal(tunnel, "burst")
		require.NoError(t, err)
		done := deliverBurstInternal(tunnel, cap(pending.ResponseCh)+1)
		<-done
		require.Len(t, pending.failureCh, 1)
		_, still := tunnel.Pending.Load("burst")
		require.False(t, still, "a timed-out request is removed")
	})
}
