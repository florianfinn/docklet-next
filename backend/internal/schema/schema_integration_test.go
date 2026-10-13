//go:build postgres_integration

// Source: docklet-next/docs/design/core-adoption.md
// Revision: be847e36ab81fdf0cd6a7dad333046ddea990e71
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Original issue #8 fresh database and bidirectional model schema proof.

package schema

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

func freshDatabase(t *testing.T, admin *gorm.DB, source *url.URL) string {
	t.Helper()
	var random [8]byte
	_, err := rand.Read(random[:])
	require.NoError(t, err)
	name := "docklet_schema_" + hex.EncodeToString(random[:])
	require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
	t.Cleanup(func() { require.NoError(t, admin.Exec("DROP DATABASE "+name+" WITH (FORCE)").Error) })
	copyURL := *source
	copyURL.Path = "/" + name
	return copyURL.String()
}

func catalog(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	const query = `
SELECT object FROM (
 SELECT 'table:' || c.relname AS object FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname='public' AND c.relkind='r' AND c.relname<>'goose_db_version'
 UNION ALL
 SELECT 'column:' || c.relname || '.' || a.attname || ':' || a.attnum || ':' || format_type(a.atttypid,a.atttypmod) || ':' || a.attnotnull || ':' || coalesce(pg_get_expr(d.adbin,d.adrelid),'')
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid
 LEFT JOIN pg_attrdef d ON d.adrelid=c.oid AND d.adnum=a.attnum
 WHERE n.nspname='public' AND c.relkind='r' AND c.relname<>'goose_db_version' AND a.attnum>0 AND NOT a.attisdropped
 UNION ALL
 SELECT 'index:' || pg_get_indexdef(i.indexrelid) FROM pg_index i JOIN pg_class c ON c.oid=i.indrelid JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname='public' AND c.relname<>'goose_db_version'
 UNION ALL
 SELECT 'constraint:' || c.relname || ':' || x.conname || ':' || pg_get_constraintdef(x.oid)
 FROM pg_constraint x JOIN pg_class c ON c.oid=x.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname='public' AND c.relname<>'goose_db_version'
 UNION ALL
 SELECT 'sequence:' || c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind='S' AND c.relname<>'goose_db_version_id_seq'
 UNION ALL
 SELECT 'enum:' || t.typname || ':' || e.enumlabel FROM pg_type t JOIN pg_namespace n ON n.oid=t.typnamespace JOIN pg_enum e ON e.enumtypid=t.oid WHERE n.nspname='public'
) objects ORDER BY object`
	var rows []string
	require.NoError(t, db.Raw(query).Scan(&rows).Error)
	return rows
}

func TestFreshMigrationMatchesRetainedModels(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	require.NotEmpty(t, dsn, "DATABASE_URL must point to the test PostgreSQL service")
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(u.Scheme, "postgres"))
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminSQL.Close() })
	migratedURL := freshDatabase(t, admin, u)
	modelURL := freshDatabase(t, admin, u)
	migrated, err := database.Initialize(t.Context(), migratedURL, database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, migrated.Close()) })
	modelDB, err := gorm.Open(postgres.Open(modelURL), &gorm.Config{IgnoreRelationshipsWhenMigrating: true})
	require.NoError(t, err)
	modelSQL, err := modelDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, modelSQL.Close()) })
	require.NoError(t, modelDB.AutoMigrate(Models()...))
	require.Equal(t, catalog(t, modelDB), catalog(t, migrated.DB))

	var tableNames []string
	require.NoError(t, migrated.Raw("SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename<>'goose_db_version' ORDER BY tablename").Scan(&tableNames).Error)
	require.Len(t, tableNames, len(Models()))
	for _, name := range tableNames {
		var count int64
		require.NoError(t, migrated.Raw(fmt.Sprintf("SELECT count(*) FROM public.%q", name)).Scan(&count).Error)
		require.Zero(t, count, "baseline must contain no seed rows in %s", name)
	}
}
