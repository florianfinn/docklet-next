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

	"github.com/stretchr/testify/require"
)

func deliverBurstInternal(tunnel *AgentTunnel, chunks int) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		server := &TunnelServer{}
		for i := range chunks {
			kind := MessageTypeCommandOutput
			if i == chunks-1 {
				kind = MessageTypeCommandComplete
			}
			server.deliverResponse(context.Background(), tunnel, &TunnelMessage{ID: "burst", Type: kind, Body: []byte("x"), Sequence: int64(i)})
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
		for i := range chunks {
			msg := <-pending.ResponseCh
			require.Equal(t, int64(i), msg.Sequence, "chunks stay in order")
			require.Equal(t, i == chunks-1, msg.Type == MessageTypeCommandComplete, "terminal last")
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
		<-pending.done
		synctest.Wait()
		require.Len(t, pending.failureCh, 1)
		require.True(t, pending.deliveryFailed())
		_, still := tunnel.Pending.Load("burst")
		require.False(t, still, "a timed-out request is removed")
	})
}
