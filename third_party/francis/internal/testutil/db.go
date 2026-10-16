// Source: italypaleale/francis/internal/testutil/db.go
// Revision: 23cd46831a543a73eec05ad034afd46245225876
// Retrieved: 2026-10-16
// License: MIT
// Deviations: SQLite-only temporary file helper; PostgreSQL helpers omitted; default test build.

package testutil

import (
	"path/filepath"
	"testing"
)

// SQLiteConnString creates a connection string for an in-memory SQLite DB that's unique for the test
func SQLiteConnString(t *testing.T) string {
	t.Helper()

	// Give the test database a unique name to make sure each test connects to its own DB
	dbName := filepath.Join(t.TempDir(), "provider.db")

	// Return the connection string
	return "file:" + dbName + ""
}
