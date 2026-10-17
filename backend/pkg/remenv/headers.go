// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package remenv

import "net/http"

const AgentContentSecurityPolicy = "sandbox; default-src 'none'"

// StripHubCredentials runs before the target agent token is installed.
func StripHubCredentials(headers http.Header, proxyIdentityHeader string) {
	for name := range headers {
		switch http.CanonicalHeaderKey(name) {
		case "Authorization", "Cookie", "X-Api-Key", "X-Arcane-Agent-Token", "Remote-User", http.CanonicalHeaderKey(proxyIdentityHeader):
			delete(headers, name)
		}
	}
}

// CopyAgentResponseHeaders confines agent-controlled content at the hub origin.
func CopyAgentResponseHeaders(from, to http.Header, hop map[string]struct{}) {
	for name, values := range from {
		canonical := http.CanonicalHeaderKey(name)
		if _, skipped := hop[canonical]; skipped {
			continue
		}
		switch canonical {
		case "Content-Type", "Content-Length", "Content-Disposition", "Content-Encoding", "Content-Range", "Accept-Ranges", "Cache-Control", "Etag", "Last-Modified":
			to[canonical] = append([]string(nil), values...)
		}
	}
	to.Set("X-Content-Type-Options", "nosniff")
	to.Set("Content-Security-Policy", AgentContentSecurityPolicy)
}
