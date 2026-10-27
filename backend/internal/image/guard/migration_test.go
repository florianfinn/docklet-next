// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

package guard

import (
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/backend/resources"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestImageProtectionForwardMigrationAndReopen(t *testing.T) {
	raw := sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db"))
	localPath, err := sqlitestore.Path(raw)
	require.NoError(t, err)
	pool, err := sqlitestore.Open(t.Context(), localPath, false, false)
	require.NoError(t, err)
	files, err := fs.Sub(resources.FS, "migrations/sqlite")
	require.NoError(t, err)
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, files)
	require.NoError(t, err)
	_, err = provider.UpTo(t.Context(), 5)
	require.NoError(t, err)
	_, err = pool.ExecContext(t.Context(), "INSERT INTO kv(key,value) VALUES('retained','unchanged')")
	require.NoError(t, err)
	require.NoError(t, pool.Close())
	db, err := database.Initialize(t.Context(), raw, database.MigrationOptions{})
	require.NoError(t, err)
	pin := Protection{ImageID: imageA, Reason: "restore-point", OwnerID: "point"}
	require.NoError(t, db.Create(&pin).Error)
	require.Error(t, db.Create(&Protection{ImageID: imageB, Reason: "invalid", OwnerID: "bad"}).Error)
	require.NoError(t, db.Close())
	db, err = database.Initialize(t.Context(), raw, database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	var pins []Protection
	require.NoError(t, db.Find(&pins).Error)
	require.Equal(t, []Protection{pin}, pins)
	var value string
	require.NoError(t, db.Raw("SELECT value FROM kv WHERE key='retained'").Scan(&value).Error)
	require.Equal(t, "unchanged", value)
}
