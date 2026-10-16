//go:build postgres_integration

// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original interrupted-migration recovery test for issue #32.

package backup

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

func TestRecoveryAcceptsIntermediateTargetVersionOnlyWithMarker(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	require.NotEmpty(t, dsn)
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	adminPool, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adminPool.Close()) })
	var suffix [8]byte
	_, err = rand.Read(suffix[:])
	require.NoError(t, err)
	name := "docklet_recovery_" + hex.EncodeToString(suffix[:])
	require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
	t.Cleanup(func() { require.NoError(t, admin.Exec("DROP DATABASE "+name+" WITH (FORCE)").Error) })
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	parsed.Path = "/" + name
	db, err := database.Initialize(t.Context(), parsed.String(), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	newest, err := newestMigration()
	require.NoError(t, err)
	require.Greater(t, newest, int64(1))
	require.ErrorIs(t, validateTargetVersion(t.Context(), db, newest-1, newest, false), errMigration)
	require.NoError(t, validateTargetVersion(t.Context(), db, newest-1, newest, true))
	require.ErrorIs(t, validateTargetVersion(t.Context(), db, newest+1, newest, true), errMigration)
}
