// Source: docklet-next/issues/288
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization; M16 bind mounts and backup completion.

package environment

import (
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEdgeReregistrationBumpsRevisionButReconnectDoesNot(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	before, err := f.service.GetEnvironmentByID(t.Context(), f.id)
	require.NoError(t, err)
	require.NoError(t, f.service.linkEnvironmentApiKey(t.Context(), f.id, "registration-key", "test-only", "operator", "operator", before.Name))
	paired, err := f.service.GetEnvironmentByID(t.Context(), f.id)
	require.NoError(t, err)
	require.Equal(t, before.CredentialRevision+1, paired.CredentialRevision)
	require.NoError(t, f.service.UpdateEnvironmentConnectionState(t.Context(), f.id, true))
	require.NoError(t, f.service.UpdateEnvironmentConnectionState(t.Context(), f.id, false))
	reconnected, err := f.service.GetEnvironmentByID(t.Context(), f.id)
	require.NoError(t, err)
	require.Equal(t, paired.CredentialRevision, reconnected.CredentialRevision)
}
