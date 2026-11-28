// Source: docklet-next/issues/694
// Revision: b60de6b21438bce9e847ef055f7ad1fd158f4c9e
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original session-expiry test for finite log reads, from the acceptance-fix review.

package ws

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/stretchr/testify/require"
)

// A finite read rechecks the session on the transport cadence; output after expiry never arrives.
func TestRuntimeLogsFiniteReadEndsOnSessionExpiry(t *testing.T) {
	var expired atomic.Bool
	produce := make(chan struct{})
	joined := make(chan struct{})
	entered := make(chan struct{})
	h := &WebSocketHandler{containerLogStreamer: func(ctx context.Context, _ string, out chan<- string, _ bool, _, _, _ string, _ bool) error {
		defer close(joined)
		close(entered)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-produce:
		}
		select {
		case out <- "unauthorised post-expiry output":
		case <-ctx.Done():
			return ctx.Err()
		}
		<-ctx.Done()
		return ctx.Err()
	}}
	decorate := func(r *http.Request) *http.Request {
		ctx := sessionlife.WithSession(r.Context(), "example-session", "example-user", func(context.Context) error {
			if expired.Load() {
				return errors.New("expired")
			}
			return nil
		})
		ctx = sessionlife.WithCadence(ctx, func(time.Duration, bool) sessionlife.Cadence {
			ticker := time.NewTicker(time.Millisecond)
			return sessionlife.Cadence{C: ticker.C, Stop: ticker.Stop, Reset: ticker.Reset}
		})
		return r.WithContext(ctx)
	}
	conn := dialLog(t, logServer(t, h, true, decorate)+"containers/example/logs?format=json&follow=false")
	defer func() { _ = conn.CloseNow(); <-joined }()
	<-entered
	expired.Store(true)
	time.Sleep(30 * time.Millisecond)
	close(produce)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	require.Error(t, err, "expired session received: %s", data)
	require.NoError(t, ctx.Err())
}
