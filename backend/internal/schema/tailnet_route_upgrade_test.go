// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original upgrade proof for migration 007 from the shipped 006 schema.

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

// Existing hosts keep their rows, tokens and revision triggers; the route stays NULL (legacy, no guess).
func TestTailnetRouteMigrationKeepsShippedHosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	pool, err := sqlitestore.Open(t.Context(), path, true, false)
	require.NoError(t, err)
	files, err := fs.Sub(resources.FS, "migrations/sqlite")
	require.NoError(t, err)
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, files)
	require.NoError(t, err)
	_, err = provider.UpTo(t.Context(), 6)
	require.NoError(t, err)
	for _, statement := range []string{
		`INSERT INTO api_keys(id,name,key_hash,key_prefix) VALUES('key-1','Example','hash','prefix')`,
		`INSERT INTO environments(id,name,api_url,status,enabled,is_edge,access_token,api_key_id,credential_revision,status_check_revision,created_at) VALUES('edge-host','Edge','edge://edge-host','online',1,1,'example-token','key-1',3,2,CURRENT_TIMESTAMP)`,
		`INSERT INTO environments(id,name,api_url,status,enabled,is_edge,credential_revision,created_at) VALUES('direct-host','Direct','http://192.0.2.10:3552','offline',1,0,0,CURRENT_TIMESTAMP)`,
	} {
		_, err = pool.ExecContext(t.Context(), statement)
		require.NoError(t, err)
	}
	before := sqlRows(t, pool, "SELECT id,name,api_url,status,enabled,is_edge,access_token,api_key_id,credential_revision,status_check_revision FROM environments ORDER BY id")
	triggersBefore := sqlRows(t, pool, "SELECT name FROM sqlite_master WHERE type='trigger' AND tbl_name='environments' ORDER BY name")
	require.NoError(t, pool.Close())

	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	actual, err := db.SQLDB()
	require.NoError(t, err)
	require.Equal(t, before, sqlRows(t, actual, "SELECT id,name,api_url,status,enabled,is_edge,access_token,api_key_id,credential_revision,status_check_revision FROM environments ORDER BY id"))
	require.Equal(t, [][]string{{"NULL", "NULL", "NULL", "NULL"}, {"NULL", "NULL", "NULL", "NULL"}},
		sqlRows(t, actual, "SELECT coalesce(network_route,'NULL'),coalesce(tailnet_node_id,'NULL'),coalesce(tailnet_tagged,'NULL'),coalesce(tailnet_observed_at,'NULL') FROM environments ORDER BY id"))
	require.Equal(t, triggersBefore, sqlRows(t, actual, "SELECT name FROM sqlite_master WHERE type='trigger' AND tbl_name='environments' ORDER BY name"))
	require.NotEmpty(t, triggersBefore)

	_, err = actual.ExecContext(t.Context(), "UPDATE environments SET network_route='tailnet', tailnet_tagged=1 WHERE id='edge-host'")
	require.NoError(t, err)
	_, err = actual.ExecContext(t.Context(), "UPDATE environments SET network_route='vpn' WHERE id='edge-host'")
	require.Error(t, err, "unknown routes are refused by the column check")
	_, err = actual.ExecContext(t.Context(), "UPDATE environments SET credential_revision=2 WHERE id='edge-host'")
	require.ErrorContains(t, err, "host.status-verdict", "the revision trigger still guards the table")
}
