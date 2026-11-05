// Source: docklet-next/issues/319
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original nested volume-policy live-zone projection proof.

package policies

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
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"gorm.io/gorm"
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

func TestVolumePolicyUsesLiveZone(t *testing.T) {
	db, e := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, e)
	require.NoError(t, db.AutoMigrate(&VolumeBackupPolicy{}))
	pool, e := db.DB()
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	cfg, save := persistedZoneForTest(t)
	service := NewService(Dependencies{DB: &database.DB{DB: db}, LatestRun: noLatestRunInternal, Location: time.UTC, ZoneProvider: cfg})
	first, e := service.GetBackupPolicies(t.Context(), "app-data")
	require.NoError(t, e)
	require.Equal(t, "UTC", first.TimeZone)
	save("Asia/Tokyo")
	after, e := service.GetBackupPolicies(t.Context(), "app-data")
	require.NoError(t, e)
	require.Equal(t, "Asia/Tokyo", after.TimeZone)
}
