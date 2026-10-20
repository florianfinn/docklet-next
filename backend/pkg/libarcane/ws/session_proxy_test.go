// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original real Direct rejection and failed-dial cleanup regression.

package ws

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/stretchr/testify/require"
)

func TestSessionDirectRejectionAndDialFailure(t *testing.T) {
	for _, rejected := range []bool{true, false} {
		t.Run(map[bool]string{true: "rejected", false: "dial-failure"}[rejected], func(t *testing.T) {
			var dials atomic.Int32
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { dials.Add(1); w.WriteHeader(502) }))
			defer remote.Close()
			done := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(done)
				ctx := sessionlife.WithSession(r.Context(), "example-failed-proxy", "example-user", func(context.Context) error {
					if rejected {
						return errors.New("revoked")
					}
					return nil
				})
				_ = ProxyHTTP(w, r.WithContext(ctx), "ws"+strings.TrimPrefix(remote.URL, "http"), nil, func(*http.Request) bool { return true })
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer conn.CloseNow()
			_, _, err = conn.Read(ctx)
			require.Error(t, err)
			require.NoError(t, ctx.Err())
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("failed viewer handler did not join")
			}
			require.EqualValues(t, map[bool]int{true: 0, false: 1}[rejected], dials.Load())
			sessionlife.Revoke("example-failed-proxy", "", "")
		})
	}
}
