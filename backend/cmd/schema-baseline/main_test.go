// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Generator refuses existing files and a live hub's lifetime lock.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
)

func TestGeneratorRefusesExistingFileWithoutChangingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.db")
	require.NoError(t, os.WriteFile(path, []byte("unchanged"), 0640))
	t.Setenv("DATABASE_URL", sqlitestore.URL(path))
	require.ErrorContains(t, generate(), "requires a new database file")
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "unchanged", string(contents))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0640), info.Mode().Perm())
}

func TestGeneratorRefusesLiveHub(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hub.db")
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	t.Setenv("DATABASE_URL", sqlitestore.URL(path))
	require.ErrorContains(t, generate(), "hub.already_running")
	pool, err := db.SQLDB()
	require.NoError(t, err)
	require.NoError(t, pool.Ping())
}
