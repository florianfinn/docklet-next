// Source: docklet-next/issues/680
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age implementation.

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
func TestMinimumAgeMigrationKeepsStoredObservations(t *testing.T) {
	for _, hub := range []bool{false, true} {
		t.Run(map[bool]string{false: "agent", true: "hub"}[hub], func(t *testing.T) {
			initialize := database.Initialize
			if hub {
				initialize = database.InitializeHub
			}
			path := filepath.Join(t.TempDir(), "upgrade.db")
			pool, err := sqlitestore.Open(t.Context(), path, true, false)
			require.NoError(t, err)
			files, err := fs.Sub(resources.FS, "migrations/sqlite")
			require.NoError(t, err)
			provider, err := goose.NewProvider(goose.DialectSQLite3, pool, files)
			require.NoError(t, err)
			_, err = provider.UpTo(t.Context(), 8)
			require.NoError(t, err)
			_, err = pool.ExecContext(t.Context(), `INSERT INTO image_updates(id,check_time,repository,tag,current_version,latest_digest,has_update) VALUES('sha256:example',CURRENT_TIMESTAMP,'example.com/app','latest','latest','sha256:next',1)`)
			require.NoError(t, err)
			before := sqlRows(t, pool, "SELECT id,repository,tag,current_version,latest_digest,has_update FROM image_updates")
			require.NoError(t, pool.Close())

			db, err := initialize(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
			require.NoError(t, err)
			require.True(t, db.Migrator().HasColumn("image_updates", "latest_manifest"))
			require.True(t, db.Migrator().HasTable("image_update_first_seen"))
			actual, err := db.SQLDB()
			require.NoError(t, err)
			require.Equal(t, before, sqlRows(t, actual, "SELECT id,repository,tag,current_version,latest_digest,has_update FROM image_updates"))
			require.Equal(t, [][]string{{"NULL"}}, sqlRows(t, actual, "SELECT coalesce(latest_manifest,'NULL') FROM image_updates"))
			_, err = actual.ExecContext(t.Context(), "INSERT INTO image_update_first_seen VALUES('example.com/app','linux/amd64','manifest',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)")
			require.NoError(t, err)
			require.NoError(t, db.Close())
			db, err = initialize(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
			require.NoError(t, err)
			defer db.Close()
			var count int64
			require.NoError(t, db.Table("image_update_first_seen").Count(&count).Error)
			require.EqualValues(t, 1, count)
		})
	}
}
