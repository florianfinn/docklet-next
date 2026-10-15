//go:build postgres_integration

// Source: docklet-next/backend/internal/database/database.go
// Revision: b3f21c5fdbe83c4c2b3549b8b0bc97a790470b2b
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Regression tests for legacy database rejection and forward raw-output migration.

package database

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRejectsLegacyGooseDatabase(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	require.NotEmpty(t, dsn, "DATABASE_URL must point to the test PostgreSQL service")
	source, err := url.Parse(dsn)
	require.NoError(t, err)
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adminSQL.Close()) })

	for _, version := range []int64{1, 90} {
		t.Run(strconv.FormatInt(version, 10), func(t *testing.T) {
			var random [8]byte
			_, err := rand.Read(random[:])
			require.NoError(t, err)
			name := "docklet_legacy_" + hex.EncodeToString(random[:])
			require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
			t.Cleanup(func() { require.NoError(t, admin.Exec("DROP DATABASE "+name+" WITH (FORCE)").Error) })
			local := *source
			local.Path = "/" + name
			conn, err := gorm.Open(postgres.Open(local.String()), &gorm.Config{})
			require.NoError(t, err)
			db, err := conn.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			require.NoError(t, migrateDatabaseToVersionInternal(t.Context(), db, dbProviderPostgres, MigrationOptions{}, 1))
			_, err = db.ExecContext(t.Context(), `ALTER TABLE public.users ADD COLUMN roles jsonb`)
			require.NoError(t, err)
			if version > 1 {
				_, err = db.ExecContext(t.Context(), `INSERT INTO goose_db_version (version_id, is_applied) VALUES ($1, true)`, version)
				require.NoError(t, err)
			}
			err = migrateDatabaseToVersionInternal(t.Context(), db, dbProviderPostgres, MigrationOptions{}, 1)
			require.ErrorContains(t, err, "legacy databases are not supported")
		})
	}
}

func TestRawOutputMigrationUpgradesExistingActivityRows(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	require.NotEmpty(t, dsn, "DATABASE_URL must point to the test PostgreSQL service")
	source, err := url.Parse(dsn)
	require.NoError(t, err)
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adminSQL.Close()) })
	var random [8]byte
	_, err = rand.Read(random[:])
	require.NoError(t, err)
	name := "docklet_output_" + hex.EncodeToString(random[:])
	require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
	t.Cleanup(func() { require.NoError(t, admin.Exec("DROP DATABASE "+name+" WITH (FORCE)").Error) })
	local := *source
	local.Path = "/" + name
	conn, err := gorm.Open(postgres.Open(local.String()), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := conn.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, migrateDatabaseToVersionInternal(t.Context(), sqlDB, dbProviderPostgres, MigrationOptions{}, 6))
	require.NoError(t, conn.Exec(`INSERT INTO activities (id, created_at, environment_id, type, status, started_at) VALUES ('existing', NOW(), '0', 'image_pull', 'success', NOW())`).Error)
	require.NoError(t, migrateDatabaseToVersionInternal(t.Context(), sqlDB, dbProviderPostgres, MigrationOptions{}, 7))
	var output string
	var truncated bool
	require.NoError(t, conn.Raw(`SELECT output::text, output_truncated FROM activities WHERE id = 'existing'`).Row().Scan(&output, &truncated))
	require.Equal(t, "[]", output)
	require.False(t, truncated)
}
