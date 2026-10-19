// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: File-backed baseline identity, refusal and restart tests.

package database

import (
	"fmt"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/backend/resources"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
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
		{"newer", "INSERT INTO goose_db_version(version_id,is_applied) VALUES(999,1)", true, "newer"},
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
	migrations := fstest.MapFS{}
	embedded, err := fs.Sub(resources.FS, "migrations/sqlite")
	require.NoError(t, err)
	entries, err := fs.ReadDir(embedded, ".")
	require.NoError(t, err)
	for _, entry := range entries {
		data, readErr := fs.ReadFile(embedded, entry.Name())
		require.NoError(t, readErr)
		migrations[entry.Name()] = &fstest.MapFile{Data: data}
	}
	highest, err := getHighestEmbeddedMigrationVersionInternal(dbProviderSQLite)
	require.NoError(t, err)
	migrations[fmt.Sprintf("%03d_fixture.sql", highest+1)] = &fstest.MapFile{Data: []byte("-- +goose Up\nALTER TABLE kv ADD COLUMN forward_value TEXT;\n")}
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

func TestRuntimeMigrationUpgradesRealSQLiteHistory(t *testing.T) {
	url := sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db"))
	db, err := connectDatabaseInternal(t.Context(), url)
	require.NoError(t, err)
	pool, err := db.SQLDB()
	require.NoError(t, err)
	// Apply the actual shipped bytes, not a reduced model fixture.
	require.NoError(t, migrateDatabaseToVersionInternal(t.Context(), pool, dbProviderSQLite, MigrationOptions{}, 2))
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 123456000, time.UTC)
	for _, statement := range []string{
		"INSERT INTO kv(key,value,created_at) VALUES('retained','value',1760000000000000)",
		"INSERT INTO activities(id,created_at,environment_id,type,status,started_at,output,output_truncated) VALUES('retained-run',1760000000000000,'0','container_start','success',1760000000000000,'[{\"text\":\"retained\"}]',1)",
		"INSERT INTO activity_messages(id,created_at,activity_id,level,message) VALUES('retained-message',1760000000000001,'retained-run','info','retained output')",
		"INSERT INTO activity_sync_receipts(run_id,updated_at) VALUES('retained-sync',1760000000000002)",
		"INSERT INTO system_backup_policies(id,enabled,schedule,retention_count) VALUES('retained-policy',1,'0 0 * * *',3)",
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	var oldCatalog string
	require.NoError(t, db.Raw("SELECT group_concat(sql,';') FROM (SELECT sql FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' AND name <> 'goose_db_version' AND tbl_name <> 'goose_db_version' ORDER BY type,name)").Scan(&oldCatalog).Error)
	require.NoError(t, db.Close())

	var upgradedCatalog string
	for attempt := 0; attempt < 3; attempt++ {
		db, err = Initialize(t.Context(), url, MigrationOptions{})
		require.NoError(t, err)
		var actual string
		require.NoError(t, db.Raw("SELECT group_concat(sql,';') FROM (SELECT sql FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' AND name NOT LIKE 'runtime_%' AND name NOT LIKE 'idx_runtime_%' AND name <> 'goose_db_version' AND tbl_name <> 'goose_db_version' ORDER BY type,name)").Scan(&actual).Error)
		require.Equal(t, oldCatalog, actual, "003 must leave all existing schema objects intact")
		require.NoError(t, db.Raw("SELECT group_concat(sql,';') FROM (SELECT sql FROM sqlite_master ORDER BY type,name)").Scan(&actual).Error)
		if attempt == 0 {
			upgradedCatalog = actual
			for _, table := range []string{"runtime_receipts", "runtime_key_bindings", "runtime_store_generation"} {
				var count int64
				require.NoError(t, db.Table(table).Count(&count).Error)
				require.Zero(t, count, "migration must leave application tables empty: %s", table)
			}
			// Seed a generation only as test data; migration creates none.
			require.NoError(t, db.Exec("INSERT INTO runtime_store_generation(id,generation,created_at) VALUES(1,'fixture-generation',?)", stamp).Error)
			require.NoError(t, db.Exec("INSERT INTO runtime_receipts(run_id,kind,store_generation,manager_id,registration_id,environment_id,executor_generation,request_digest,caller_id,operation_id,target_kind,target_id,gate_scope,gate_key,admitted_at,dispatch_state,executor_state,cancel_requested,release_acknowledged,final_write_at) VALUES('compact-run','receipt','fixture-generation','manager','registration','0','executor-generation','digest','caller','start-container','container','container-id','container:container-id','start:digest',?,'pending','pending',0,0,?)", stamp, stamp).Error)
			require.NoError(t, db.Exec("INSERT INTO runtime_key_bindings(environment_id,idempotency_key,run_id,caller_id,request_digest,first_seen_at) VALUES('0','opaque-key','compact-run','joining-caller','digest',?)", stamp).Error)
			require.NoError(t, db.Exec("INSERT INTO runtime_receipts(run_id,kind,store_generation,manager_id,registration_id,environment_id,executor_generation,request_digest,cancel_requested,release_acknowledged,settled_at,final_write_at) VALUES('compact-negative','not_received','fixture-generation','manager','registration','0','fixture-generation','negative-digest',0,0,?,?)", stamp, stamp).Error)
		}
		require.Equal(t, upgradedCatalog, actual, "repeat initialization must not alter the current schema")
		var value, output, message, identity string
		var truncated bool
		require.NoError(t, db.Raw("SELECT value FROM kv WHERE key='retained'").Scan(&value).Error)
		require.Equal(t, "value", value)
		require.NoError(t, db.Raw("SELECT output,output_truncated FROM activities WHERE id='retained-run'").Row().Scan(&output, &truncated))
		require.JSONEq(t, `[{"text":"retained"}]`, output)
		require.True(t, truncated)
		require.NoError(t, db.Raw("SELECT message FROM activity_messages WHERE id='retained-message'").Scan(&message).Error)
		require.Equal(t, "retained output", message)
		var synced int64
		require.NoError(t, db.Raw("SELECT CAST(updated_at AS INTEGER) FROM activity_sync_receipts WHERE run_id='retained-sync'").Scan(&synced).Error)
		require.EqualValues(t, 1760000000000002, synced)
		var enabled bool
		var retention int
		require.NoError(t, db.Raw("SELECT enabled,retention_count FROM system_backup_policies WHERE id='retained-policy'").Row().Scan(&enabled, &retention))
		require.True(t, enabled)
		require.Equal(t, 3, retention)
		require.NoError(t, db.Raw("SELECT identity FROM docklet_database_metadata").Scan(&identity).Error)
		require.Equal(t, "docklet-next:sqlite:1", identity)
		var generation string
		var created time.Time
		require.NoError(t, db.Raw("SELECT generation,created_at FROM runtime_store_generation WHERE id=1").Row().Scan(&generation, &created))
		require.Equal(t, "fixture-generation", generation)
		require.Equal(t, stamp, created)
		var caller, digest, executor, joiningCaller string
		require.NoError(t, db.Raw("SELECT caller_id,request_digest,executor_generation FROM runtime_receipts WHERE run_id='compact-run'").Row().Scan(&caller, &digest, &executor))
		require.Equal(t, "caller", caller)
		require.Equal(t, "digest", digest)
		require.Equal(t, "executor-generation", executor)
		require.NoError(t, db.Raw("SELECT caller_id,first_seen_at FROM runtime_key_bindings WHERE environment_id='0' AND idempotency_key='opaque-key'").Row().Scan(&joiningCaller, &created))
		require.Equal(t, "joining-caller", joiningCaller)
		require.Equal(t, stamp, created)
		var kind string
		var admission *time.Time
		require.NoError(t, db.Raw("SELECT kind,admitted_at FROM runtime_receipts WHERE run_id='compact-negative'").Row().Scan(&kind, &admission))
		require.Equal(t, "not_received", kind)
		require.Nil(t, admission)
		var integrity string
		require.NoError(t, db.Raw("PRAGMA integrity_check").Scan(&integrity).Error)
		require.Equal(t, "ok", integrity)
		rows, queryErr := db.Raw("PRAGMA foreign_key_check").Rows()
		require.NoError(t, queryErr)
		require.False(t, rows.Next())
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		require.NoError(t, db.Close())
	}
}
