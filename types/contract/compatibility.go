// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original compatibility boundary; optional-module foundation (#472). M36 #568: historical v20 fingerprint.

package contract

// AgentHubV18Fingerprint preserves the supported pre-module contract boundary.
const AgentHubV18Fingerprint = "425ca5dd725baecb39fc5d66bebde3eb260296f9344255eb4dc79c9fd0fac182"

// AgentHubV19Fingerprint preserves baseline compatibility without new module requests.
const AgentHubV19Fingerprint = "cd658728da4e2de1b4d1c625544b9ef2e48bad0bfba685f7d11cc1651e4d5414"

// AgentHubV20Fingerprint preserves the 1.3.0 agent surface; v21 only adds the deploy.point-no-baseline reason.
const AgentHubV20Fingerprint = "51489456b62b53b9128a6c61a7d5285c341631dcc5fbedb44c87da8d5e8f54fd"

// AgentCompatible rejects older contracts and a changed schema at the same version.
func AgentCompatible(version int, fingerprint string) bool {
	if version == 20 {
		return fingerprint == AgentHubV20Fingerprint
	}
	if version == 19 {
		return fingerprint == AgentHubV19Fingerprint
	}
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
	// Proxy module requests changed in contract 20 (M28: resolvers, certificate check,
	// operation kind), so v19 agents get update-required for every module request.
	return (version == 20 && fingerprint == AgentHubV20Fingerprint) || (version >= 21 && fingerprint == AgentHubFingerprint)
}
