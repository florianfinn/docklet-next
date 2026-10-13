//go:build postgres_integration

// Source: getarcaneapp/arcane/backend/pkg/scheduler/analytics_job_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: retained shared test setup after removing upstream analytics; functional cut: PostgreSQL integration test fixture.

package scheduler

import (
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/testpg"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
)

func setupSchedulerStateServicesInternal(t *testing.T) (*database.DB, *settings.SettingsService, *kv.KVService) {
	t.Helper()
	ctx := t.Context()
	db, err := gorm.Open(testpg.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&settings.SettingVariable{}))
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))

	wrappedDB := &database.DB{DB: db}
	settingsService, err := newSettingsServiceForTestInternal(t, ctx, wrappedDB)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "instanceId", "test-instance"))

	return wrappedDB, settingsService, kv.NewKVService(wrappedDB)
}
