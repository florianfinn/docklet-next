// Source: docklet-next/issues/369
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original system protection with explicit successful nonmatching current-process identity.

package container

import (
	c "github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSystemEffectGuardIndependentOfCurrentIdentity(t *testing.T) {
	for _, role := range []string{"agent", "hub"} {
		err := effectOriginGuardWithIdentity(c.InspectResponse{ID: "target", Config: &c.Config{Labels: map[string]string{"io.docklet.system": role}}}, "other-process", nil)
		require.ErrorContains(t, err, "container.origin-locked", "system role must protect a non-self workload")
	}
	require.NoError(t, effectOriginGuardWithIdentity(c.InspectResponse{ID: "target", Config: &c.Config{}}, "other-process", nil))
	require.NoError(t, effectOriginGuardWithIdentity(c.InspectResponse{ID: "target", Config: &c.Config{Labels: map[string]string{"com.getarcaneapp.arcane": "true"}}}, "other-process", nil), "Arcane non-self ID remains unprotected")
}
