// Source: getarcaneapp/arcane/backend/internal/activity/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-16
// License: BSD-3-Clause
// Deviations: Full SQLite baseline fixture with production connection and lifetime-lock policy.

package activity

import (
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func setupActivityServiceTestDBInternal(t *testing.T) *database.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "hub.db")
	db, err := database.Initialize(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}
