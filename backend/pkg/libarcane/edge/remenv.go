// Source: getarcaneapp/arcane/backend/pkg/libarcane/edge/remenv.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; retain browser cookies at the hub boundary.

package edge

import (
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
)

const (
	HeaderAPIKey        = "X-Api-Key" // #nosec G101: header name, not a credential
	HeaderAuthorization = "Authorization"
	HeaderCookie        = "Cookie"
	HeaderAgentToken    = "X-Arcane-Agent-Token" // #nosec G101: header name, not a credential
	HeaderUpgrade       = "Upgrade"
	HeaderConnection    = "Connection"

	ConnectionUpgradeToken = "upgrade"
)

func CopyRequestHeaders(from, to http.Header, skip map[string]struct{}) {
	for k, vs := range from {
		ck := http.CanonicalHeaderKey(k)
		if _, ok := skip[ck]; ok || ck == http.CanonicalHeaderKey(HeaderAuthorization) || ck == http.CanonicalHeaderKey(HeaderAPIKey) || ck == "Remote-User" || ck == HeaderAgentToken {
			continue
		}
		for _, v := range vs {
			to.Add(k, v)
		}
	}
}

func SetAuthHeader(req *http.Request, _ *echo.Context) {
	remenv.StripHubCredentials(req.Header, "Remote-User")
}

func SetAgentToken(req *http.Request, accessToken *string) {
	remenv.ApplyAgentTokenHeaders(req.Header, accessToken)
}

// BuildWebSocketHeaders sends only the target agent credential.
func BuildWebSocketHeaders(_ *echo.Context, accessToken *string) http.Header {
	headers := http.Header{}
	remenv.ApplyAgentTokenHeaders(headers, accessToken)
	return headers
}

// HTTPToWebSocketURL converts an HTTP(S) URL to WS(S).
func HTTPToWebSocketURL(url string) string {
	switch {
	case strings.HasPrefix(url, "https://"):
		return "wss://" + strings.TrimPrefix(url, "https://")
	case strings.HasPrefix(url, "http://"):
		return "ws://" + strings.TrimPrefix(url, "http://")
	default:
		return url
	}
}

// CopyBodyWithFlush streams bytes from body to w, flushing when supported.
// Useful for progress/streaming endpoints where incremental delivery matters.
func CopyBodyWithFlush(w http.ResponseWriter, body io.Reader) {
	buf := make([]byte, 32*1024)
	flusher, canFlush := w.(http.Flusher)

	for {
		n, readErr := body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if readErr != nil {
			return
		}
	}
}

func SetForwardedHeaders(req *http.Request, clientIP, host string) {
	req.Header.Set("X-Forwarded-For", clientIP)
	req.Header.Set("X-Forwarded-Host", host)
}

func GetHopByHopHeaders() map[string]struct{} {
	return map[string]struct{}{
		http.CanonicalHeaderKey("Connection"):          {},
		http.CanonicalHeaderKey("Keep-Alive"):          {},
		http.CanonicalHeaderKey("Proxy-Authenticate"):  {},
		http.CanonicalHeaderKey("Proxy-Authorization"): {},
		http.CanonicalHeaderKey("TE"):                  {},
		http.CanonicalHeaderKey("Trailers"):            {},
		http.CanonicalHeaderKey("Trailer"):             {},
		http.CanonicalHeaderKey("Transfer-Encoding"):   {},
		http.CanonicalHeaderKey("Upgrade"):             {},
	}
}

func BuildHopByHopHeaders(respHeader http.Header) map[string]struct{} {
	hop := GetHopByHopHeaders()

	for _, connVal := range respHeader.Values("Connection") {
		for token := range strings.SplitSeq(connVal, ",") {
			if t := strings.TrimSpace(token); t != "" {
				hop[http.CanonicalHeaderKey(t)] = struct{}{}
			}
		}
	}

	return hop
}

func CopyResponseHeaders(from, to http.Header, hop map[string]struct{}) {
	remenv.CopyAgentResponseHeaders(from, to, hop)
}

func GetSkipHeaders() map[string]struct{} {
	return map[string]struct{}{
		"Host": {}, "Connection": {}, "Keep-Alive": {}, "Proxy-Authenticate": {},
		"Proxy-Authorization": {}, "Te": {}, "Trailer": {}, "Transfer-Encoding": {},
		"Upgrade": {}, "Content-Length": {}, "Accept-Encoding": {}, "Origin": {}, "Referer": {},
		"Access-Control-Request-Method": {}, "Access-Control-Request-Headers": {}, "Cookie": {},
	}
}

// AgentRequestHeaders removes browser credentials while retaining the injected agent token.
func AgentRequestHeaders(source http.Header) http.Header {
	headers := source.Clone()
	token := source.Get(HeaderAgentToken)
	remenv.StripHubCredentials(headers, "Remote-User")
	remenv.ApplyAgentTokenHeaders(headers, &token)
	return headers
}
