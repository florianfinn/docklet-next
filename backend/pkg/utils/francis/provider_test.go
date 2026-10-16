// Source: getarcaneapp/arcane/backend/pkg/utils/francis/provider_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: functional cut: removed SQLite actor storage tests; functional cut: removed SQLite runtime and migrations.

package francis

import (
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStoreURL(t *testing.T) {
	for _, input := range []string{"postgres://example.invalid/db", "file::memory:", "file:x.db?vfs=unix-dotfile", ""} {
		_, err := StoreURL(input)
		require.Error(t, err)
	}
	path := filepath.Join(t.TempDir(), "hub.db")
	actual, err := StoreURL(sqlitestore.URL(path))
	require.NoError(t, err)
	require.Equal(t, sqlitestore.URL(filepath.Join(filepath.Dir(path), "hub.francis.db")), actual)
}
