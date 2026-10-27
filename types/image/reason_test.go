// Source: docklet-next/docs/design/update-policies.md
// Revision: 8907719aba6cd2d3037d287729f689d5ad9e3ec9
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Runtime reason rejection, polling freshness and durable host permission refusals.

package image

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestValidateReasonRejectsEveryUndeclaredValue(t *testing.T) {
	require.NoError(t, ValidateReason(""))
	for _, code := range strings.Split(ReasonEnum, ",") {
		require.NoError(t, ValidateReason(code))
	}
	for _, code := range []string{"FORBIDDEN", "BOGUS", "update.undeclared", " update.permission-denied", "update.permission-denied "} {
		require.ErrorIs(t, ValidateReason(code), ErrUndeclaredReason)
	}
}
