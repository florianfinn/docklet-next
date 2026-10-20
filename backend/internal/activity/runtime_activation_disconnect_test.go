// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Real HTTP lost-response detachment with owned SDK work on all transports.

package activity_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestRuntimeActivationNetworkDisconnectKeepsExecution(t *testing.T) {
	for _, transport := range []string{"local", "direct", "edge"} {
		t.Run(transport, func(t *testing.T) {
			joined := make(chan struct{})
			hub := activationApp(t, false, "", activationSetup{BeforeRouter: func(router *echo.Echo) {
				router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
					return func(c *echo.Context) error {
						if c.Request().Method == "POST" && strings.HasSuffix(c.Request().URL.Path, "/start") && c.Request().Header.Get("Prefer") == "" {
							defer close(joined)
						}
						return next(c)
					}
				})
			}})
			executor, env := hub, "0"
			if transport != "local" {
				executor = activationApp(t, true, uuid.NewString())
				env = activationRemote(t, hub, executor, transport)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			executor.services.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
				if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/start") {
					once.Do(func() { close(entered) })
					<-release
					w.WriteHeader(204)
					return true
				}
				return false
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			path, key := "/api/environments/"+env+"/containers/"+runtimeContainerID+"/start", uuid.NewString()
			request, err := http.NewRequestWithContext(ctx, "POST", hub.server.URL+path, nil)
			require.NoError(t, err)
			request.Header.Set("Idempotency-Key", key)
			clientDone := make(chan error, 1)
			go func() {
				response, err := hub.server.Client().Do(request)
				if response != nil {
					_, _ = io.Copy(io.Discard, response.Body)
					_ = response.Body.Close()
				}
				clientDone <- err
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("owned SDK child did not start")
			}
			cancel()
			select {
			case err := <-clientDone:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(5 * time.Second):
				t.Fatal("network viewer did not detach")
			}
			select {
			case <-joined:
			case <-time.After(5 * time.Second):
				t.Fatal("actual disconnected HTTP handler did not join")
			}
			status, body := activationCall(t, hub, path, key, "respond-async", nil)
			require.Equal(t, 200, status, string(body))
			run := activationRunID(t, body)
			wait, stop := context.WithTimeout(t.Context(), 5*time.Second)
			defer stop()
			require.NoError(t, hub.remote.WaitDispatch(wait, run))
			var receipt activity.RuntimeReceipt
			require.NoError(t, executor.services.db.First(&receipt, "run_id = ?", run).Error)
			require.Nil(t, receipt.SettledAt)
			require.False(t, receipt.CancelRequested)
			status, body = activationCall(t, hub, strings.TrimSuffix(path, "start")+"stop", uuid.NewString(), "respond-async", nil)
			require.Equal(t, 409, status, string(body))
			require.Contains(t, string(body), run)
			unblock()
			activationJoinRun(t, hub, executor, env, run)
			require.Len(t, runtimeEffectEvents(executor.services.http.snapshot()), 1)
		})
	}
}
