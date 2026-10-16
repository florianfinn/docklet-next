// Source: getarcaneapp/arcane/backend/pkg/libarcane/edge/client_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Relocated forwarding regression; validate exact configured origin or explicit agent credential.

package edge

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/coder/websocket"
	httputil "github.com/florianfinn/docklet-next/backend/pkg/utils/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTunnelClient_DialLocalWebSocket_StripsForwardedBrowserHeaders(t *testing.T) {
	managerURL := "https://manager.internal.example.com"

	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "agent-token" {
			http.Error(w, "missing agent auth", http.StatusForbidden)
			return
		}

		if origin := r.Header.Get("Origin"); origin != "" {
			http.Error(w, "unexpected forwarded origin: "+origin, http.StatusForbidden)
			return
		}

		if cookie := r.Header.Get("Cookie"); cookie != "" {
			http.Error(w, "unexpected forwarded cookie", http.StatusForbidden)
			return
		}

		if !httputil.ValidateWebSocketOrigin([]string{managerURL}, func(req *http.Request) bool {
			return req.Header.Get("X-API-Key") == "agent-token"
		})(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		if !assert.NoError(t, conn.Write(r.Context(), websocket.MessageText, []byte("ok"))) {
			return
		}
	}))
	defer localServer.Close()

	parsedURL, err := url.Parse(localServer.URL)
	require.NoError(t, err)

	client := NewTunnelClient(&Config{
		AgentToken: "agent-token",
		Listen:     parsedURL.Hostname(),
		Port:       parsedURL.Port(),
	}, http.NotFoundHandler())

	msg := &TunnelMessage{
		Path: "/",
		Headers: map[string]string{
			"Host":              "manager.example.com",
			"Origin":            "https://public.browser.example.com",
			"Cookie":            "session=browser-cookie",
			"Authorization":     "Bearer browser-token",
			"Sec-Fetch-Mode":    "websocket",
			"Sec-Fetch-Site":    "same-origin",
			"Sec-Websocket-Key": "forwarded-handshake-key",
		},
	}

	headers := client.buildLocalWebSocketHeadersInternal(msg)
	assert.Empty(t, headers.Get("Host"))
	assert.Empty(t, headers.Get("Origin"))
	assert.Empty(t, headers.Get("Cookie"))
	assert.Empty(t, headers.Get("Authorization"))

	ws, _, err := client.dialLocalWebSocket(t.Context(), client.buildLocalWebSocketURLInternal(msg), headers)
	require.NoError(t, err)
	defer func() { _ = ws.CloseNow() }()

	msgType, body, err := ws.Read(t.Context())
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageText, msgType)
	assert.Equal(t, "ok", string(body))
}
