// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original compatibility boundary; M26 same-version fingerprint boundary.

package contract

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAgentCompatibilityAndRecoveryRoutes(t *testing.T) {
	require.True(t, AgentCompatible(AgentHubVersion, AgentHubFingerprint))
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

// Builds before the M26 collection routes share version 18 and must refuse this build.
func TestEarlierV18FingerprintIsRejected(t *testing.T) {
	require.False(t, AgentCompatible(18, "7e7dbc378cf7da65372c3ff4bce98ec958b98e009f481b98b0140c4497d9744a"))
}
