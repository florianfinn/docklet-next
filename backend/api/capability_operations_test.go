// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original D6 host producer vocabulary regression proof.

package api

import (
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOperationCapabilityHostStatuses(t *testing.T) {
	for _, tc := range []struct {
		status environment.EnvironmentStatus
		reason string
	}{
		{environment.EnvironmentStatusTokenRejected, "agent.invalid-token"},
		{environment.EnvironmentStatusUpdateRequired, "agent.update-required"},
		{environment.EnvironmentStatusOffline, "agent.unavailable"},
		{environment.EnvironmentStatusOnline, ""},
		{environment.EnvironmentStatusStandby, ""},
	} {
		t.Run(string(tc.status), func(t *testing.T) { require.Equal(t, tc.reason, operationHostStatus(string(tc.status))) })
	}
}
