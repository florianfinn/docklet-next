//go:build postgres_integration

// Source: getarcaneapp/arcane/backend/pkg/scheduler/scheduled_prune_job_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: PostgreSQL integration test fixture.

package scheduler

import (
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/testpg"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
)

func setupScheduledPruneSettingsServiceInternal(t *testing.T) *settings.SettingsService {
	t.Helper()

	db, err := gorm.Open(testpg.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&settings.SettingVariable{}))

	svc, err := newSettingsServiceForTestInternal(t, t.Context(), &database.DB{DB: db})
	require.NoError(t, err)

	return svc
}

func TestBuildScheduledPruneRequestInternal(t *testing.T) {
	ctx := t.Context()
	settingsService := setupScheduledPruneSettingsServiceInternal(t)

	require.NoError(t, settingsService.UpdateSetting(ctx, "pruneContainerMode", "olderThan"))
	require.NoError(t, settingsService.UpdateSetting(ctx, "pruneContainerUntil", "24h"))
	require.NoError(t, settingsService.UpdateSetting(ctx, "pruneImageMode", "all"))
	require.NoError(t, settingsService.UpdateSetting(ctx, "pruneVolumeMode", "anonymous"))
	require.NoError(t, settingsService.UpdateSetting(ctx, "pruneNetworkMode", "none"))
	require.NoError(t, settingsService.UpdateSetting(ctx, "pruneBuildCacheMode", "olderThan"))
	require.NoError(t, settingsService.UpdateSetting(ctx, "pruneBuildCacheUntil", "30m"))

	req := buildScheduledPruneRequestInternal(ctx, settingsService)

	require.Equal(t, &system.PruneContainersOptions{
		Mode:  system.PruneContainerModeOlderThan,
		Until: "24h",
	}, req.Containers)
	require.Equal(t, &system.PruneImagesOptions{
		Mode: system.PruneImageModeAll,
	}, req.Images)
	require.Equal(t, &system.PruneVolumesOptions{
		Mode: system.PruneVolumeModeAnonymous,
	}, req.Volumes)
	require.Nil(t, req.Networks)
	require.Equal(t, &system.PruneBuildCacheOptions{
		Mode:  system.PruneBuildCacheModeOlderThan,
		Until: "30m",
	}, req.BuildCache)
	require.True(t, hasScheduledPruneTargetsInternal(req))
}

func TestBuildScheduledPruneRequestInternal_SkipsWhenAllModesAreNone(t *testing.T) {
	ctx := t.Context()
	settingsService := setupScheduledPruneSettingsServiceInternal(t)

	require.NoError(t, settingsService.UpdateSetting(ctx, "pruneContainerMode", "none"))
	require.NoError(t, settingsService.UpdateSetting(ctx, "pruneImageMode", "none"))
	require.NoError(t, settingsService.UpdateSetting(ctx, "pruneVolumeMode", "none"))
	require.NoError(t, settingsService.UpdateSetting(ctx, "pruneNetworkMode", "none"))
	require.NoError(t, settingsService.UpdateSetting(ctx, "pruneBuildCacheMode", "none"))

	req := buildScheduledPruneRequestInternal(ctx, settingsService)

	require.Nil(t, req.Containers)
	require.Nil(t, req.Images)
	require.Nil(t, req.Volumes)
	require.Nil(t, req.Networks)
	require.Nil(t, req.BuildCache)
	require.False(t, hasScheduledPruneTargetsInternal(req))
}
