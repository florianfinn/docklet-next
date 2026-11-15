// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original compatibility boundary; optional-module foundation (#472).

package contract

// AgentHubV18Fingerprint preserves the supported pre-module contract boundary.
const AgentHubV18Fingerprint = "425ca5dd725baecb39fc5d66bebde3eb260296f9344255eb4dc79c9fd0fac182"

// AgentCompatible rejects older contracts and a changed schema at the same version.
func AgentCompatible(version int, fingerprint string) bool {
	if version == 18 {
		return version >= MinimumAgentVersion && fingerprint == AgentHubV18Fingerprint
	}
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

// ModuleCompatible is stricter than the baseline v18 host surface.
func ModuleCompatible(version int, fingerprint string) bool {
	return version >= 19 && fingerprint == AgentHubFingerprint
}
