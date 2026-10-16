// Source: getarcaneapp/arcane/backend/internal/database/database_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; fresh SQLite migration coverage; SQLite-only local persistence.

package database

import (
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedSQLiteMigrationsAreOrdered(t *testing.T) {
	versions, err := getEmbeddedMigrationVersionsInternal(dbProviderSQLite)
	require.NoError(t, err)
	require.NotEmpty(t, versions)
	for i := 1; i < len(versions); i++ {
		require.Greater(t, versions[i], versions[i-1])
	}
	highest, err := getHighestEmbeddedMigrationVersionInternal(dbProviderSQLite)
	require.NoError(t, err)
	require.Equal(t, versions[len(versions)-1], highest)
}

func TestDatabaseRejectsNonSQLiteURL(t *testing.T) {
	_, err := connectDatabaseInternal(t.Context(), "postgres://example.invalid/db")
	require.ErrorContains(t, err, "file:")
	_, err = connectDatabaseInternal(t.Context(), "mysql://localhost/test")
	require.ErrorContains(t, err, "file:")
}

func TestGooseDialectOnlyAcceptsSQLite(t *testing.T) {
	dialect, err := gooseDialectInternal(dbProviderSQLite)
	require.NoError(t, err)
	require.Equal(t, goose.DialectSQLite3, dialect)
	_, err = gooseDialectInternal("unsupported")
	require.Error(t, err)
}
