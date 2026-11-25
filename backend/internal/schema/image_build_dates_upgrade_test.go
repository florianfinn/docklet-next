// Source: docklet-next/issues/634
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original upgrade proof for migration 008 from the shipped 007 schema.

package schema

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/backend/resources"
)

// Stored update observations keep their rows; build times stay NULL until the next check.
func TestImageBuildDatesMigrationKeepsStoredObservations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	pool, err := sqlitestore.Open(t.Context(), path, true, false)
	require.NoError(t, err)
	files, err := fs.Sub(resources.FS, "migrations/sqlite")
	require.NoError(t, err)
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, files)
	require.NoError(t, err)
	_, err = provider.UpTo(t.Context(), 7)
	require.NoError(t, err)
	_, err = pool.ExecContext(t.Context(), `INSERT INTO image_updates(id,check_time,repository,tag,current_version,latest_digest,has_update) VALUES('sha256:example',CURRENT_TIMESTAMP,'example.com/app','latest','latest','sha256:next',1)`)
	require.NoError(t, err)
	before := sqlRows(t, pool, "SELECT id,repository,tag,current_version,latest_digest,has_update FROM image_updates")
	require.NoError(t, pool.Close())

	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	actual, err := db.SQLDB()
	require.NoError(t, err)
	require.Equal(t, before, sqlRows(t, actual, "SELECT id,repository,tag,current_version,latest_digest,has_update FROM image_updates"))
	require.Equal(t, [][]string{{"NULL", "NULL"}}, sqlRows(t, actual, "SELECT coalesce(current_created,'NULL'),coalesce(latest_created,'NULL') FROM image_updates"))
	_, err = actual.ExecContext(t.Context(), "UPDATE image_updates SET current_created='2026-11-12 08:00:00+00:00', latest_created='2026-11-24 06:00:00+00:00'")
	require.NoError(t, err)
}
