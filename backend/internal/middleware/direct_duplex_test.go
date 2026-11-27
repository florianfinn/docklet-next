// Source: docklet-next/issues/672
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original Direct proxy full-duplex transport regression.

package middleware

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDirectProxyFullDuplex(t *testing.T) {
	const size = 1 << 20
	agentRead := make(chan int64, 1)
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = http.NewResponseController(w).EnableFullDuplex()
		w.Header().Set("Content-Type", "application/x-json-stream")
		fmt.Fprint(w, "start\n")
		http.NewResponseController(w).Flush()
		n, _ := io.Copy(io.Discard, r.Body)
		agentRead <- n
		fmt.Fprintf(w, "done:%d\n", n)
	}))
	defer agent.Close()
	hub := httptest.NewServer(securityDirectRouter(t, agent.URL))
	defer hub.Close()
	reader, writer := io.Pipe()
	defer reader.Close()
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	// A bounded fallback releases the sender even under the broken half-duplex path.
	// The verdict remains the returned stream and body count, never elapsed time.
	timer := time.AfterFunc(200*time.Millisecond, unblock)
	defer timer.Stop()
	sent := make(chan error, 1)
	go func() {
		_, err := io.WriteString(writer, "a")
		if err == nil {
			<-release
			_, err = io.Copy(writer, strings.NewReader(strings.Repeat("b", size-1)))
		}
		writer.Close()
		sent <- err
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", hub.URL+"/api/environments/example/images/pull", reader)
	require.NoError(t, err)
	resp, err := hub.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	first := make([]byte, len("start\n"))
	_, readErr := io.ReadFull(resp.Body, first)
	unblock()
	rest, streamErr := io.ReadAll(resp.Body)
	require.NoError(t, ctx.Err(), "stream must settle before deadline")
	require.NoError(t, readErr)
	require.Equal(t, fmt.Sprintf("start\ndone:%d\n", size), string(first)+string(rest), "truncated Direct response: %v", streamErr)
	require.NoError(t, streamErr)
	require.NoError(t, <-sent)
	require.EqualValues(t, size, <-agentRead)
}
