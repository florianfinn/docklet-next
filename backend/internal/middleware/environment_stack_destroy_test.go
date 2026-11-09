// Source: docklet-next/issues/434
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Direct and Edge stack destroy body regression tests.

package middleware

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

const destroyRoute = "/api/environments/example/projects/demo/destroy"

type destroyAgent struct {
	calls  int
	files  bool
	vols   bool
	status int
	reply  string
}

// handler decodes the forwarded body the way the agent's huma operation does:
// an empty body is a nil *Destroy, and the wire type resolves the flags.
func (a *destroyAgent) handler(t *testing.T) http.Handler {
	agent := servedRouter()
	agent.DELETE("/api/environments/:id/projects/:projectId/destroy", func(c *echo.Context) error {
		a.calls++
		var raw []byte
		if c.Request().Body != nil {
			var err error
			raw, err = io.ReadAll(c.Request().Body)
			require.NoError(t, err)
		}
		var body *projecttypes.Destroy
		if len(raw) > 0 {
			body = &projecttypes.Destroy{}
			require.NoError(t, json.Unmarshal(raw, body))
		}
		a.files, a.vols = body.Flags()
		status, reply := a.status, a.reply
		if status == 0 {
			status, reply = http.StatusOK, `{"success":true,"data":{"message":"Project destroyed successfully"}}`
		}
		return c.Blob(status, "application/json", []byte(reply))
	})
	return agent
}

func destroyRequest(body io.Reader) *http.Request {
	req := httptest.NewRequest(http.MethodDelete, destroyRoute, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestStackDestroyFlagsReachTheAgentOnDirectAndEdge(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		files, vols bool
	}{
		{"keep both", `{"removeFiles":false,"removeVolumes":false}`, false, false},
		{"remove files", `{"removeFiles":true}`, true, false},
		{"remove volumes", `{"removeFiles":false,"removeVolumes":true}`, false, true},
		{"remove both", `{"removeFiles":true,"removeVolumes":true}`, true, true},
		{"empty object keeps files", `{}`, false, false},
		{"no body keeps files", "", false, false},
	}
	for _, edgeMode := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("edge=%t/%s", edgeMode, tc.name), func(t *testing.T) {
				agent := &destroyAgent{}
				hub := servedHub(t, edgeMode, newTestEnvironmentMiddleware(), agent.handler(t))
				var body io.Reader
				if tc.body != "" {
					body = strings.NewReader(tc.body)
				}
				rec := httptest.NewRecorder()
				hub.ServeHTTP(rec, destroyRequest(body))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Equal(t, 1, agent.calls)
				require.Equal(t, tc.files, agent.files, "removeFiles")
				require.Equal(t, tc.vols, agent.vols, "removeVolumes")
			})
		}
	}
}

type failingDestroyConn struct{ servedConn }

func (c *failingDestroyConn) Send(msg *edge.TunnelMessage) error {
	if msg.Type == edge.MessageTypeCommandRequest {
		return errors.New("tunnel closed")
	}
	return nil
}

func TestStackDestroyPolicyCleanupFollowsTheAgentReply(t *testing.T) {
	type call struct{ env, stack string }
	for _, edgeMode := range []bool{false, true} {
		for _, tc := range []struct {
			name    string
			status  int
			reply   string
			cleanup []call
		}{
			{"success", 0, "", []call{{"example", "demo"}}},
			{"agent conflict", http.StatusConflict, `{"success":false,"code":"project.busy","data":{"error":"busy"}}`, nil},
			{"agent failure", http.StatusInternalServerError, `{"success":false,"code":"project.failed"}`, nil},
		} {
			t.Run(fmt.Sprintf("edge=%t/%s", edgeMode, tc.name), func(t *testing.T) {
				var calls []call
				m := newTestEnvironmentMiddleware()
				m.policyCleanup = func(_ context.Context, env, stack string) error {
					calls = append(calls, call{env, stack})
					return nil
				}
				agent := &destroyAgent{status: tc.status, reply: tc.reply}
				hub := servedHub(t, edgeMode, m, agent.handler(t))
				rec := httptest.NewRecorder()
				hub.ServeHTTP(rec, destroyRequest(strings.NewReader(`{"removeFiles":false}`)))
				require.Equal(t, tc.cleanup, calls)
				if tc.status != 0 {
					require.Equal(t, tc.status, rec.Code)
					require.JSONEq(t, tc.reply, rec.Body.String(), "typed agent error reaches the client unchanged")
				}
			})
		}
	}
	t.Run("edge tunnel error", func(t *testing.T) {
		calls := 0
		m := newTestEnvironmentMiddleware()
		m.policyCleanup = func(context.Context, string, string) error { calls++; return nil }
		hub := servedHub(t, true, m, (&destroyAgent{}).handler(t))
		conn := &failingDestroyConn{}
		tunnel := edge.NewAgentTunnelWithConn("example", conn)
		conn.tunnel = tunnel
		m.registry.Register("example", tunnel)
		rec := httptest.NewRecorder()
		hub.ServeHTTP(rec, destroyRequest(strings.NewReader(`{"removeFiles":false}`)))
		require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
		require.Zero(t, calls)
	})
}

type countingReader struct {
	r    io.Reader
	read int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += n
	return n, err
}

func TestEdgeStackDestroyBoundsTheBody(t *testing.T) {
	padded := func(size int) string {
		prefix := `{"removeFiles":true,"pad":"`
		return prefix + strings.Repeat("a", size-len(prefix)-2) + `"}`
	}
	t.Run("at the bound", func(t *testing.T) {
		agent := &destroyAgent{}
		hub := servedHub(t, true, newTestEnvironmentMiddleware(), agent.handler(t))
		rec := httptest.NewRecorder()
		hub.ServeHTTP(rec, destroyRequest(strings.NewReader(padded(stackDestroyBodyLimit))))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, 1, agent.calls)
		require.True(t, agent.files)
	})
	t.Run("over the bound", func(t *testing.T) {
		agent := &destroyAgent{}
		cleanups := 0
		m := newTestEnvironmentMiddleware()
		m.policyCleanup = func(context.Context, string, string) error { cleanups++; return nil }
		hub := servedHub(t, true, m, agent.handler(t))
		body := &countingReader{r: strings.NewReader(padded(1 << 20))}
		rec := httptest.NewRecorder()
		hub.ServeHTTP(rec, destroyRequest(body))
		require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
		var reply struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &reply))
		require.Equal(t, "request.too-large", reply.Code)
		require.Zero(t, agent.calls, "no tunnel dispatch")
		require.Zero(t, cleanups)
		require.LessOrEqual(t, body.read, stackDestroyBodyLimit+1, "reader stops at the bound")
	})
}
