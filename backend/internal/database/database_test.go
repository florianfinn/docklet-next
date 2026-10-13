// Source: getarcaneapp/arcane/backend/internal/database/database_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed SQLite migration tests; functional cut: removed SQLite runtime and migrations.

package database

import (
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedPostgresMigrationsAreOrdered(t *testing.T) {
	versions, err := getEmbeddedMigrationVersionsInternal(dbProviderPostgres)
	require.NoError(t, err)
	require.NotEmpty(t, versions)
	for i := 1; i < len(versions); i++ {
		require.Greater(t, versions[i], versions[i-1])
	}
	highest, err := getHighestEmbeddedMigrationVersionInternal(dbProviderPostgres)
	require.NoError(t, err)
	require.Equal(t, versions[len(versions)-1], highest)
}

func TestDatabaseRejectsNonPostgresURL(t *testing.T) {
	_, err := connectDatabaseInternal(t.Context(), "file:legacy.db")
	require.ErrorContains(t, err, "PostgreSQL")
	_, err = connectDatabaseInternal(t.Context(), "mysql://localhost/test")
	require.ErrorContains(t, err, "PostgreSQL")
}

func TestGooseDialectOnlyAcceptsPostgres(t *testing.T) {
	dialect, err := gooseDialectInternal(dbProviderPostgres)
	require.NoError(t, err)
	require.Equal(t, goose.DialectPostgres, dialect)
	_, err = gooseDialectInternal("unsupported")
	require.Error(t, err)
}
