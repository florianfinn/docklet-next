// Source: docklet-next/issues/330
// Revision: a37d77d05c92bde3a4eab0cf6fa47ab36411e011
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original regression for agent reads cancelled by shutdown (M16 practical acceptance DV1 D).

package bootstrap

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"

	"github.com/florianfinn/docklet-next/backend/internal/config"
)

// The handler fails like an image read whose Docker call shutdown cancelled; the agent answers that failure as
// unavailable, while the hub keeps its own answer and an agent's earlier success stays unchanged.
func TestAgentShutdownAnswersCancelledReadAsUnavailable(t *testing.T) {
	cases := []struct {
		name   string
		agent  bool
		status int
		want   []any
	}{
		{"agent failure", true, http.StatusServiceUnavailable, []any{http.StatusBadGateway, `{"success":false,"code":"agent.unavailable"}`}},
		{"agent success", true, http.StatusOK, []any{http.StatusOK, `{"success":true}`}},
		{"hub failure", false, http.StatusServiceUnavailable, []any{http.StatusServiceUnavailable, `{"code":"image.usage-unavailable","success":false}`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entered := make(chan struct{})
			router := echo.New()
			router.GET("/api/environments/0/images", func(c *echo.Context) error {
				close(entered)
				<-c.Request().Context().Done()
				if tc.status == http.StatusOK {
					return c.JSON(http.StatusOK, map[string]any{"success": true})
				}
				return c.JSON(tc.status, map[string]any{"success": false, "code": "image.usage-unavailable"})
			})
			lc := fxtest.NewLifecycle(t)
			srv, err := NewHTTPServer(lc, HTTPServerParams{AppCtx: t.Context(), Config: &config.Config{AgentMode: tc.agent, Listen: "127.0.0.1", Port: "0"}, Router: router})
			require.NoError(t, err)
			lc.RequireStart()

			type result struct {
				status int
				body   string
				err    error
			}
			done := make(chan result, 1)
			go func() {
				resp, err := http.Get("http://" + srv.Addr + "/api/environments/0/images") //nolint:noctx // bounded by the select below
				if err != nil {
					done <- result{err: err}
					return
				}
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				done <- result{resp.StatusCode, string(body), err}
			}()
			<-entered
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			require.NoError(t, lc.Stop(stopCtx))
			select {
			case got := <-done:
				require.NoError(t, got.err)
				require.Equal(t, tc.want, []any{got.status, trimNewline(got.body)})
			case <-time.After(5 * time.Second):
				t.Fatal("request did not finish after shutdown")
			}
		})
	}
}

func trimNewline(s string) string {
	for len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	return s
}
