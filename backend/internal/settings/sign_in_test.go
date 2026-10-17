// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original read-only startup policy regression test.

package settings

import (
	"github.com/florianfinn/docklet-next/backend/internal/common"
	settingstypes "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSignInModeCannotChangeThroughSettings(t *testing.T) {
	svc := &SettingsService{}
	value := "false"
	_, err := svc.UpdateSettings(t.Context(), settingstypes.Update{AuthLocalEnabled: &value})
	require.ErrorIs(t, err, common.ErrValidation)
}
