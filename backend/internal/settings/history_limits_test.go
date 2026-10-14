//go:build postgres_integration

// Source: getarcaneapp/arcane/backend/internal/settings/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: PostgreSQL history-setting bounds regression.

package settings

import (
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
)

func TestHistoryLimitsValidateAllWritePaths(t *testing.T) {
	svc, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	for _, tc := range []struct {
		key, min, max, below, above string
	}{
		{"eventHistoryMaxEntries", "50", "5000", "49", "5001"},
		{"activityHistoryMaxEntries", "10", "1000", "9", "1001"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			for _, invalid := range []string{tc.below, tc.above, "not-a-number"} {
				err := svc.UpdateSetting(t.Context(), tc.key, invalid)
				require.ErrorIs(t, err, common.ErrValidation)
				var update settings.Update
				if tc.key == "eventHistoryMaxEntries" {
					update.EventHistoryMaxEntries = &invalid
				} else {
					update.ActivityHistoryMaxEntries = &invalid
				}
				_, err = svc.UpdateSettings(t.Context(), update)
				require.ErrorIs(t, err, common.ErrValidation)
			}
			for _, valid := range []string{tc.min, tc.max} {
				require.NoError(t, svc.UpdateSetting(t.Context(), tc.key, valid))
				require.Equal(t, valid, svc.GetStringSetting(t.Context(), tc.key, ""))
			}
		})
	}
}
