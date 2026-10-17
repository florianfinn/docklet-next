// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original compatibility boundary.

package contract

// AgentCompatible rejects older contracts and a changed schema at the same version.
func AgentCompatible(version int, fingerprint string) bool {
	return version >= MinimumAgentVersion && (version != AgentHubVersion || fingerprint == AgentHubFingerprint)
}

// AgentRecoveryRoute is the complete pre-compatibility forwarding surface.
func AgentRecoveryRoute(method, path string) bool {
	if method == "HEAD" && (path == "/api/health" || path == "/api/environments/0/system/health") {
		return true
	}
	if method == "GET" {
		switch path {
		case "/api/health", "/api/app-version", "/api/version", "/api/environments/0/system/upgrade/check":
			return true
		}
	}
	return method == "POST" && path == "/api/environments/0/system/upgrade"
}
