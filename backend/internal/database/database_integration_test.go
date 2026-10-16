// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: File-backed baseline identity, refusal and restart tests.

package database

import (
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/backend/resources"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestDatabaseIdentityAndReopen(t *testing.T) {
	url := sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db"))
	db, err := Initialize(t.Context(), url, MigrationOptions{})
	require.NoError(t, err)
	var before string
	require.NoError(t, db.Raw("SELECT group_concat(sql,';') FROM sqlite_master").Scan(&before).Error)
	require.NoError(t, db.Close())
	db, err = Initialize(t.Context(), url, MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	var after string
	require.NoError(t, db.Raw("SELECT group_concat(sql,';') FROM sqlite_master").Scan(&after).Error)
	require.Equal(t, before, after)
	var output string
	var truncated bool
	require.NoError(t, db.Exec("INSERT INTO activities(id,created_at,environment_id,type,status,started_at) VALUES('existing',1700000000000000,'0','image_pull','success',?)", 1700000000000000).Error)
	require.NoError(t, db.Raw("SELECT output,output_truncated FROM activities WHERE id='existing'").Row().Scan(&output, &truncated))
	require.Equal(t, "[]", output)
	require.False(t, truncated)
}
func TestRejectsForeignAndInconsistentSQLite(t *testing.T) {
	for _, test := range []struct {
		name, sql string
		current   bool
		want      string
	}{
		{"unowned", "CREATE TABLE example(id TEXT)", false, "foreign"},
		{"legacy", "CREATE TABLE schema_migrations(version INTEGER)", false, "foreign"},
		{"arcane", "CREATE TABLE users_table(id TEXT)", false, "foreign"},
		{"missing_version", "DELETE FROM goose_db_version WHERE version_id=1", true, "inconsistent"},
		{"duplicate", "INSERT INTO goose_db_version(version_id,is_applied) VALUES(1,1)", true, "inconsistent"},
		{"downgraded", "UPDATE goose_db_version SET is_applied=0 WHERE version_id=1", true, "inconsistent"},
		{"newer", "INSERT INTO goose_db_version(version_id,is_applied) VALUES(2,1)", true, "newer"},
		{"wrong_identity", "PRAGMA ignore_check_constraints=ON; UPDATE docklet_database_metadata SET identity='other'; PRAGMA ignore_check_constraints=OFF", true, "foreign"},
		{"missing_identity", "DROP TABLE docklet_database_metadata", true, "foreign"},
	} {
		t.Run(test.name, func(t *testing.T) {
			url := sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db"))
			var db *DB
			var err error
			if test.current {
				db, err = Initialize(t.Context(), url, MigrationOptions{})
			} else {
				db, err = connectDatabaseInternal(t.Context(), url)
			}
			require.NoError(t, err)
			require.NoError(t, db.Exec(test.sql).Error)
			require.NoError(t, db.Close())
			_, err = Initialize(t.Context(), url, MigrationOptions{})
			require.ErrorContains(t, err, test.want)
			// Refusal must release the lifetime lock.
			path, err := sqlitestore.Path(url)
			require.NoError(t, err)
			lock, err := acquireHubLock(t.Context(), path)
			require.NoError(t, err)
			require.NoError(t, lock.Stop())
		})
	}
}

func TestGooseAppliesForwardUpgradeWithoutRewritingBaseline(t *testing.T) {
	url := sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db"))
	db, err := Initialize(t.Context(), url, MigrationOptions{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("INSERT INTO kv(key,value) VALUES('retained','value')").Error)
	baseline, err := resources.FS.ReadFile("migrations/sqlite/001_baseline.sql")
	require.NoError(t, err)
	migrations := fstest.MapFS{"001_baseline.sql": &fstest.MapFile{Data: baseline}, "002_fixture.sql": &fstest.MapFile{Data: []byte("-- +goose Up\nALTER TABLE kv ADD COLUMN forward_value TEXT;\n")}}
	pool, err := db.SQLDB()
	require.NoError(t, err)
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, migrations)
	require.NoError(t, err)
	_, err = provider.Up(t.Context())
	require.NoError(t, err)
	var value string
	require.NoError(t, db.Raw("SELECT value FROM kv WHERE key='retained'").Scan(&value).Error)
	require.Equal(t, "value", value)
	require.NoError(t, db.Exec("UPDATE kv SET forward_value='new' WHERE key='retained'").Error)
	require.NoError(t, db.Close())
	_, err = Initialize(t.Context(), url, MigrationOptions{})
	require.ErrorContains(t, err, "newer")
}
