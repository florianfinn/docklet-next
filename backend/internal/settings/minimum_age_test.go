// Source: docklet-next/issues/680
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age implementation.

package settings

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMinimumAgeSettingValidation(t *testing.T) {
	for _, v := range []string{"-1", "91", "1.5", "oops"} {
		require.Error(t, validateSettingValueInternal("updateMinimumAgeDays", v), v)
	}
	for _, v := range []string{"0", "90"} {
		require.NoError(t, validateSettingValueInternal("updateMinimumAgeDays", v))
	}
}
