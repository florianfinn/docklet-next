// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original compatibility boundary; M26 same-version fingerprint boundary; optional-module foundation (#472).

package contract

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAgentCompatibilityAndRecoveryRoutes(t *testing.T) {
	require.True(t, AgentCompatible(AgentHubVersion, AgentHubFingerprint))
	require.True(t, AgentCompatible(20, AgentHubV20Fingerprint), "1.3.0 agents stay supported")
	require.True(t, AgentCompatible(21, AgentHubV21Fingerprint), "1.4.0 agents stay supported")
	require.True(t, ModuleCompatible(21, AgentHubV21Fingerprint), "the module surface is unchanged since v21")
	// F3 (#592): adoption is hub-only, so v22 (1.5.0) agents keep baseline and module requests.
	require.True(t, AgentCompatible(22, AgentHubV22Fingerprint), "1.5.0 agents stay supported")
	require.True(t, ModuleCompatible(22, AgentHubV22Fingerprint), "v23 changes no agent module request")
	require.False(t, AgentCompatible(22, AgentHubFingerprint), "a changed v22 schema is refused")
	require.False(t, ModuleCompatible(23, AgentHubV22Fingerprint), "a changed v23 schema is refused")
	require.False(t, AgentCompatible(21, AgentHubFingerprint))
	require.False(t, AgentCompatible(20, AgentHubFingerprint))
	require.True(t, AgentCompatible(19, AgentHubV19Fingerprint))
	require.False(t, ModuleCompatible(19, AgentHubV19Fingerprint), "M28 changed the proxy module surface")
	require.False(t, AgentCompatible(19, AgentHubFingerprint))
	require.False(t, AgentCompatible(MinimumAgentVersion-1, AgentHubFingerprint))
	require.False(t, AgentCompatible(AgentHubVersion, "different"))
	for _, path := range []string{"/api/health", "/api/app-version", "/api/version", "/api/environments/0/system/upgrade/check"} {
		require.True(t, AgentRecoveryRoute("GET", path))
		require.False(t, AgentRecoveryRoute("DELETE", path))
	}
	require.True(t, AgentRecoveryRoute("HEAD", "/api/environments/0/system/health"))
	require.True(t, AgentRecoveryRoute("POST", "/api/environments/0/system/upgrade"))
	for _, path := range []string{"/api/environments/0/system/upgrade/all", "/api/environments/0/containers", "/api/container-registries/sync", "/api/environments/0/settings"} {
		require.False(t, AgentRecoveryRoute("POST", path))
		require.False(t, AgentRecoveryRoute("GET", path))
	}
}

func TestV8RejectsV7EvenWithCurrentFingerprint(t *testing.T) {
	require.False(t, AgentCompatible(7, AgentHubFingerprint))
}

// Changed schemas at the current v19 boundary must refuse this build.
func TestEarlierV19FingerprintIsRejected(t *testing.T) {
	require.False(t, AgentCompatible(19, "7e7dbc378cf7da65372c3ff4bce98ec958b98e009f481b98b0140c4497d9744a"))
}

// The module version bump must not admit previously rejected v18 schemas.
func TestEarlierV18FingerprintIsRejected(t *testing.T) {
	require.False(t, AgentCompatible(18, "7e7dbc378cf7da65372c3ff4bce98ec958b98e009f481b98b0140c4497d9744a"))
	require.True(t, AgentCompatible(18, AgentHubV18Fingerprint))
}
