// Source: docklet-next/issues/319
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original job projection and update-check ownership proofs.

package job

import (
	"github.com/florianfinn/docklet-next/backend/internal/config"
	dto "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestJobsUseLiveZoneAndHubCheckSchedule(t *testing.T) {
	db := setupSettingsTestDBInternal(t)
	settings, e := newSettingsServiceForTestInternal(t, t.Context(), db)
	require.NoError(t, e)
	cfg := &config.Config{Timezone: "UTC"}
	settings.BindZoneProvider(cfg)
	service := newJobServiceForTestInternal(t, db, settings, cfg)
	for _, zone := range []string{"UTC", "Asia/Tokyo"} {
		_, e = settings.UpdateSettings(t.Context(), dto.Update{TimeZone: new(zone)})
		require.NoError(t, e)
		listed, e := service.legacyListJobs(t.Context())
		require.NoError(t, e)
		found := false
		for _, job := range listed.Jobs {
			require.Equal(t, zone, job.TimeZone)
			if job.ID == "update-check" {
				found = true
				require.Equal(t, "0 0 */12 * * *", job.Schedule)
			}
		}
		require.True(t, found)
	}
	for _, edge := range []bool{false, true} {
		cfg.AgentMode = !edge
		cfg.EdgeAgent = edge
		listed, e := service.legacyListJobs(t.Context())
		require.NoError(t, e)
		for _, job := range listed.Jobs {
			require.NotEqual(t, "update-check", job.ID)
		}
	}
}
