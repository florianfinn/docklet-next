// Source: docklet-next/issues/347
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original real Edge registration/first-request ordering regression.

package edge

import (
	"context"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

type registrationBarrier struct {
	reserved, release chan struct{}
	reserveOnce       sync.Once
}

type registrationBarrierLogger struct {
	slog.Handler
	barrier *registrationBarrier
}

func (h registrationBarrierLogger) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == "Edge agent tunnel registered" {
		matches := false
		r.Attrs(func(a slog.Attr) bool {
			matches = matches || a.Key == "environmentId" && a.Value.String() == "register-first"
			return true
		})
		if matches {
			// This existing log follows the registry commit and precedes ACK Send.
			// Holding it forces the publication race without a production test hook.
			h.barrier.reserveOnce.Do(func() { close(h.barrier.reserved); <-h.barrier.release })
		}
	}
	return h.Handler.Handle(ctx, r)
}

func (h registrationBarrierLogger) WithAttrs(attrs []slog.Attr) slog.Handler {
	return registrationBarrierLogger{Handler: h.Handler.WithAttrs(attrs), barrier: h.barrier}
}

func (h registrationBarrierLogger) WithGroup(name string) slog.Handler {
	return registrationBarrierLogger{Handler: h.Handler.WithGroup(name), barrier: h.barrier}
}

func TestEdgeFirstRequestWaitsForWrittenRegisterACK(t *testing.T) {
	barrier := &registrationBarrier{reserved: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(barrier.release) })
	defer release()
	previous := slog.Default()
	previousWriter, previousFlags := log.Writer(), log.Flags()
	slog.SetDefault(slog.New(registrationBarrierLogger{Handler: slog.NewTextHandler(io.Discard, nil), barrier: barrier}))
	defer func() { slog.SetDefault(previous); log.SetOutput(previousWriter); log.SetFlags(previousFlags) }()
	registry := NewTunnelRegistry()
	server := NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) { return "register-first", nil }, nil)
	router := echo.New()
	router.GET("/api/tunnel/connect", server.HandleConnect)
	manager := httptest.NewServer(router)
	defer manager.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client := NewRuntimeTunnelClient(&Config{EdgeTransport: EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: "example", EdgeReconnectInterval: 1}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/environments/0/images" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ready":true}`))
	}))
	clientDone := make(chan struct{})
	go func() { defer close(clientDone); client.StartWithErrorChan(ctx, nil) }()
	defer func() { release(); cancel(); registry.Unregister("register-first"); <-clientDone }()
	select {
	case <-barrier.reserved:
	case <-ctx.Done():
		t.Fatal("registration reservation was not observed")
	}

	// Start the actual proxy sender immediately after registration, while the
	// manager has not started writing its ACK. It can only use published sessions.
	firstLookup := make(chan bool, 1)
	type response struct {
		code int
		body string
		err  error
	}
	finished := make(chan response, 1)
	go func() {
		tunnel, visible := registry.Get("register-first").Get()
		firstLookup <- visible
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for !visible {
			select {
			case <-ctx.Done():
				finished <- response{err: ctx.Err()}
				return
			case <-ticker.C:
				tunnel, visible = registry.Get("register-first").Get()
			}
		}
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/environments/register-first/images", nil).WithContext(ctx)
		err := ProxyHTTPRequest(echo.New().NewContext(req, recorder), tunnel, "/api/environments/0/images")
		finished <- response{code: recorder.Code, body: recorder.Body.String(), err: err}
	}()
	visibleBeforeACK := <-firstLookup
	release()
	require.False(t, visibleBeforeACK, "session was published before its register acknowledgement was written")
	var result response
	select {
	case result = <-finished:
	case <-ctx.Done():
		t.Fatal("first request did not settle")
	}
	require.NoError(t, result.err)
	require.Equal(t, http.StatusOK, result.code, result.body)
	require.JSONEq(t, `{"ready":true}`, result.body)
}
