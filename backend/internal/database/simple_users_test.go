// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package database

import (
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestSimpleUsersMigrationPreservesM14IdentityData(t *testing.T) {
	for _, mode := range []string{"hub", "agent"} {
		t.Run(mode, func(t *testing.T) {
			db, err := connectDatabaseInternal(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "fixture.db")))
			require.NoError(t, err)
			defer db.Close()
			pool, err := db.SQLDB()
			require.NoError(t, err)
			require.NoError(t, migrateDatabaseToVersionInternal(t.Context(), pool, dbProviderSQLite, MigrationOptions{}, 14))
			require.NoError(t, db.Exec("INSERT INTO users(id,username,password_hash,time_format) VALUES('operator','operator','retained','auto')").Error)
			require.NoError(t, db.Exec("INSERT INTO roles(id,name,permissions,built_in) VALUES('role_admin','Administrator','[\"users:read\"]',1)").Error)
			require.NoError(t, db.Exec("INSERT INTO user_role_assignments(id,user_id,role_id,source) VALUES('assignment','operator','role_admin','manual')").Error)
			require.NoError(t, db.Exec("INSERT INTO api_keys(id,name,key_hash,key_prefix,kind,user_id,managed_by) VALUES('static','Static','retained-hash','example','scoped','operator','admin_account_default_api_key')").Error)
			var before string
			require.NoError(t, db.Raw("SELECT group_concat(id || ':' || permissions) FROM roles").Scan(&before).Error)
			require.NoError(t, db.Migrate(t.Context()))
			var staticHash string
			require.NoError(t, db.Raw("SELECT key_hash FROM api_keys WHERE id='static'").Scan(&staticHash).Error)
			require.Equal(t, "retained-hash", staticHash)
			var after string
			require.NoError(t, db.Raw("SELECT group_concat(id || ':' || permissions) FROM roles").Scan(&after).Error)
			require.Equal(t, before, after)
			var username, hash string
			require.NoError(t, db.Raw("SELECT username,password_hash FROM users WHERE id='operator'").Row().Scan(&username, &hash))
			require.Equal(t, "operator", username)
			require.Equal(t, "retained", hash)
			var count int64
			require.NoError(t, db.Table("user_app_assignments").Count(&count).Error)
			require.Zero(t, count)
			require.NoError(t, db.Exec("INSERT INTO user_app_assignments(id,user_id,environment_id,project_id,level,created_at) VALUES('app','operator','0','uuid','restricted',CURRENT_TIMESTAMP)").Error)
			require.Error(t, db.Exec("INSERT INTO user_app_assignments(id,user_id,environment_id,project_id,level,created_at) VALUES('invalid','operator','0','other','admin',CURRENT_TIMESTAMP)").Error)
			require.NoError(t, db.Exec("DELETE FROM users WHERE id='operator'").Error)
			require.NoError(t, db.Table("user_app_assignments").Count(&count).Error)
			require.Zero(t, count)
		})
	}
}
