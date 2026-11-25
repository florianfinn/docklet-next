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

// AgentHubV21Fingerprint is the 1.4.0 surface; v22 only adds hub reads. A v21 agent lacks the candidate run read: Direct
// answers an uncoded 404 and Edge refuses the command (the hub reports a tunnel failure); the web shows the run without counts.
const AgentHubV21Fingerprint = "081a36566f1e1c401e071059756a35b8843e74d52c980a5172e455405f3f793d"

// AgentHubV22Fingerprint is the 1.5.0 surface. v23 adds hub-only proxy adoption, the binding detail and the eligibility read:
// a v22 agent sends no binding detail and has no eligibility read, so the web shows today's generic refusal text.
const AgentHubV22Fingerprint = "c2b15889bef9d47a22c686d268c49119e581536584c9fd0b20bff61d304e1ee4"

// AgentHubV23Fingerprint is the 1.6.0/1.7.0 surface. v24 adds hub-side check plans, target names and the
// missing-local-image reason; a v23 agent reports no plan, so its local job runs show no counts.
const AgentHubV23Fingerprint = "7f484ee06162f4c9fa320d0d7f22c5be1931c8aa1654970b7c19d0ede5b4a12b"

// AgentCompatible rejects older contracts and a changed schema at the same version.
func AgentCompatible(version int, fingerprint string) bool {
	if version == 23 {
		return fingerprint == AgentHubV23Fingerprint
	}
	if version == 22 {
		return fingerprint == AgentHubV22Fingerprint
	}
	if version == 21 {
		return fingerprint == AgentHubV21Fingerprint
	}
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
	return (version == 20 && fingerprint == AgentHubV20Fingerprint) || (version == 21 && fingerprint == AgentHubV21Fingerprint) || (version == 22 && fingerprint == AgentHubV22Fingerprint) || (version == 23 && fingerprint == AgentHubV23Fingerprint) || (version >= 24 && fingerprint == AgentHubFingerprint)
}
