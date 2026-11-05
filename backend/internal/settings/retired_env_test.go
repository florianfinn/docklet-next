// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original retired variable detection proof.

package settings

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRetiredSettingVariablesAreDetected(t *testing.T) {
	for _, name := range RetiredSettingVariables {
		t.Setenv(name, "")
	}
	require.Empty(t, retiredSettingVariablesInternal())
	t.Setenv("AUTO_UPDATE", "true")
	t.Setenv("VULNERABILITY_SCAN_ENABLED", "false")
	require.Equal(t, []string{"AUTO_UPDATE", "VULNERABILITY_SCAN_ENABLED"}, retiredSettingVariablesInternal())
}
