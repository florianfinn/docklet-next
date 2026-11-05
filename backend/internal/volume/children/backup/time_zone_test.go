// Source: docklet-next/issues/319
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original owning-host overview live-zone proof.

package backup

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	dto "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"net/http"
)

func persistedZoneForTest(t *testing.T) (*config.Config, func(string)) {
	t.Helper()
	db, e := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "settings.db")), database.MigrationOptions{})
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	settingsService, e := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, settingsService.Stop(context.WithoutCancel(t.Context()))) })
	cfg := &config.Config{Timezone: "UTC"}
	settingsService.BindZoneProvider(cfg)
	return cfg, func(zone string) {
		_, e := settingsService.UpdateSettings(t.Context(), dto.Update{TimeZone: new(zone)})
		require.NoError(t, e)
	}
}

func TestBackupOverviewUsesLiveZone(t *testing.T) {
	started := []string{}
	service := startupService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"Volumes":[]}`)) }), &started)
	require.NoError(t, service.deps.DB.AutoMigrate(&checkSetting{}, &backuptarget.Target{}, &policies.VolumeBackupPolicy{}))
	require.NoError(t, service.deps.DB.AutoMigrate(activity.Models()...))
	cfg, save := persistedZoneForTest(t)
	service.deps.ZoneProvider = cfg
	first, e := service.Overview(t.Context())
	require.NoError(t, e)
	require.Equal(t, "UTC", first.TimeZone)
	save("Asia/Tokyo")
	after, e := service.Overview(t.Context())
	require.NoError(t, e)
	require.Equal(t, "Asia/Tokyo", after.TimeZone)
}
