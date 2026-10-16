// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original environment-only recovery surface test.

package cli

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAdministratorRecoveryHasNoCLI(t *testing.T) {
	_, _, err := rootCmd.Find([]string{"admin", "reset-password"})
	require.Error(t, err)
}
