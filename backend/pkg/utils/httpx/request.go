// Source: getarcaneapp/arcane/backend/pkg/utils/httpx/request.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; exact configured WebSocket origins with validated non-cookie exceptions.

package httpx

import (
	"cmp"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	httpxtypes "github.com/florianfinn/docklet-next/types/httpx"
)

type HeaderSetter interface {
	SetHeader(key, value string)
}

func SetJSONStreamHeaders(headers HeaderSetter) {
	headers.SetHeader("Content-Type", "application/x-json-stream")
	SetStreamHeaders(headers)
}

func SetStreamHeaders(headers HeaderSetter) {
	// no-store keeps proxies from caching or collapsing identical concurrent
	// stream requests (e.g. two browser tabs), which would otherwise leave only
	// the first connection attached to the live upstream stream.
	headers.SetHeader("Cache-Control", "no-store, no-cache, must-revalidate")
	headers.SetHeader("Connection", "keep-alive")
	headers.SetHeader("X-Accel-Buffering", "no")
}

// ValidateWebSocketOrigin uses the same exact origin list as HTTP browser protection.
// Missing Origin requires an independently validated non-cookie credential.
func ValidateWebSocketOrigin(allowedOrigins []string, credentialAuthenticated func(*http.Request) bool) func(r *http.Request) bool {
	return func(r *http.Request) bool {
		if len(r.Header.Values("Origin")) > 1 {
			return false
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			return len(r.Header.Values("Cookie")) == 0 && credentialAuthenticated != nil && credentialAuthenticated(r)
		}
		for _, allowed := range allowedOrigins {
			if origin == allowed {
				return true
			}
		}
		return false
	}
}

// IsWebSocketUpgradeRequest reports whether r is a WebSocket handshake: the
// Connection header contains "upgrade" (it can be a list, e.g. Firefox sends
// "keep-alive, Upgrade") and Upgrade is "websocket".
func IsWebSocketUpgradeRequest(r *http.Request) bool {
	connection := strings.ToLower(r.Header.Get("Connection"))
	upgrade := strings.ToLower(r.Header.Get("Upgrade"))

	switch {
	case !strings.Contains(connection, "upgrade"):
		return false
	case upgrade != "websocket":
		return false
	default:
		return true
	}
}

// GetIntQueryParam reads and parses an integer query parameter from the request URL.
// If `required` is true and the parameter is missing, or if parsing fails, an error is returned.
func GetIntQueryParam(r *http.Request, name string, required bool) (int, error) {
	q := r.URL.Query()
	if !q.Has(name) || q.Get(name) == "" {
		if required {
			return 0, fmt.Errorf("missing numeric query parameter %s", name)
		}
		return 0, nil
	}
	n, err := strconv.Atoi(q.Get(name))
	if err != nil {
		return 0, fmt.Errorf("invalid numeric query parameter %s: %w", name, err)
	}
	return n, nil
}

// GetClientBaseURL determines the client's base URL from request headers.
// It checks Origin, X-Forwarded-Host/Proto, and Host headers.
// If none provide a valid URL, it falls back to the configured appURL.
func GetClientBaseURL(origin, forwardedHost, forwardedProto, host, appURL string) string {
	// 1. Trust Origin if present
	if origin != "" {
		return strings.TrimSuffix(origin, "/")
	}

	scheme := "http"
	// Try to get scheme from AppURL as default
	if u, err := url.Parse(appURL); err == nil && u.Scheme != "" {
		scheme = u.Scheme
	}

	scheme = cmp.Or(forwardedProto, scheme)

	// 2. Check X-Forwarded-Host
	if forwardedHost != "" {
		return fmt.Sprintf("%s://%s", scheme, forwardedHost)
	}

	// 3. Check Host
	if host != "" {
		return fmt.Sprintf("%s://%s", scheme, host)
	}

	// 4. Fallback
	return strings.TrimSuffix(appURL, "/")
}

// NewHTTPClient builds a client with its own transport and the supplied timeouts.
func NewHTTPClient(options httpxtypes.ClientOptions) *http.Client {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   options.TLSHandshakeTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   options.Timeout,
	}
}
