// Source: docklet-next/issues/358
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original upgrade from M19 with preservation and removed-feature cleanup proof.

package schema

import (
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/backend/resources"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestM20UpgradeRemovesWebhooksPreservesRolesAndRestartEvidence(t *testing.T) {
	for _, hub := range []bool{false, true} {
		t.Run(map[bool]string{true: "hub", false: "agent"}[hub], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "upgrade.db")
			pool, err := sqlitestore.Open(t.Context(), path, true, false)
			require.NoError(t, err)
			files, err := fs.Sub(resources.FS, "migrations/sqlite")
			require.NoError(t, err)
			provider, err := goose.NewProvider(goose.DialectSQLite3, pool, files)
			require.NoError(t, err)
			_, err = provider.UpTo(t.Context(), 19)
			require.NoError(t, err)
			for _, statement := range []string{
				`INSERT INTO webhooks(id,name,token_hash,token_prefix,target_type,target_id) VALUES('hook','Example','hash','prefix','container','container')`,
				`INSERT INTO api_keys(id,name,key_hash,key_prefix) VALUES('key','Example','hash','prefix')`,
				`INSERT INTO api_key_permissions(id,api_key_id,permission) VALUES('removed','key','webhooks:list'),('kept','key','containers:list')`,
				`INSERT INTO roles(id,name,permissions) VALUES('mixed','Mixed','["webhooks:list","containers:list"]'),('only','Only','["webhooks:create"]')`,
				`INSERT INTO users(id,username,password_hash) VALUES('operator','operator','hash')`,
				`INSERT INTO user_role_assignments(id,user_id,role_id) VALUES('assignment','operator','only')`,
				`INSERT INTO events(id,type,title,timestamp) VALUES('removed','webhook.trigger','Example',CURRENT_TIMESTAMP),('kept','container.start','Example',CURRENT_TIMESTAMP)`,
				`INSERT INTO settings(key,value) VALUES('webhooksCategory',''),('loadAlertsEnabled','true')`,
				`INSERT INTO agent_restart_requests(run_id,environment_id,registration_id,credential_revision,old_instance,state,created_at,updated_at,sent_at,acknowledged_at,code,deadline,settled_at) VALUES('restart','example-host','registration',7,'old','failed','2026-11-01 12:00:00','2026-11-01 12:00:01','2026-11-01 12:00:00','2026-11-01 12:00:01','host.unreachable','2026-11-01 12:01:00','2026-11-01 12:00:01')`,
			} {
				_, err = pool.ExecContext(t.Context(), statement)
				require.NoError(t, err)
			}
			before := sqlRows(t, pool, "SELECT * FROM agent_restart_requests")
			require.NoError(t, pool.Close())
			initialize := database.Initialize
			if hub {
				initialize = database.InitializeHub
			}
			db, err := initialize(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
			require.NoError(t, err)
			defer db.Close()
			actual, err := db.SQLDB()
			require.NoError(t, err)
			require.False(t, db.Migrator().HasTable("webhooks"))
			require.Equal(t, before, sqlRows(t, actual, "SELECT * FROM agent_restart_requests"), "all restart columns survive the rebuild")
			require.Equal(t, [][]string{{"containers:list"}}, sqlRows(t, actual, "SELECT permission FROM api_key_permissions WHERE api_key_id='key'"))
			require.Equal(t, [][]string{{"container.start"}}, sqlRows(t, actual, "SELECT type FROM events"))
			require.Equal(t, [][]string{{"only"}}, sqlRows(t, actual, "SELECT role_id FROM user_role_assignments WHERE id='assignment'"))
			svc := role.NewRoleService(db)
			for id, expected := range map[string][]string{"mixed": {"containers:list"}, "only": {}} {
				row, err := svc.GetRole(t.Context(), id)
				require.NoError(t, err)
				permissions := []string(row.Permissions)
				_, err = svc.UpdateRole(t.Context(), row.ID, row.Name, row.Description, permissions)
				require.NoError(t, err, "an upgraded custom role must remain saveable")
				require.Equal(t, expected, permissions)
			}
			cfg, err := settings.NewSettingsService(t.Context(), db)
			require.NoError(t, err)
			defer cfg.Stop(t.Context())
			require.NoError(t, cfg.PruneUnknownSettings(t.Context()), "the startup pruner removes the retired placeholder")
			require.Equal(t, [][]string{{"0"}}, sqlRows(t, actual, "SELECT count(*) FROM settings WHERE key='webhooksCategory'"))
			require.Equal(t, [][]string{{"true"}}, sqlRows(t, actual, "SELECT value FROM settings WHERE key='loadAlertsEnabled'"))
			code := "host.busy"
			now := time.Now().UTC()
			busy := environment.AgentRestartRequest{RunID: "busy", EnvironmentID: "example-host", RegistrationID: "registration", CredentialRevision: 7, OldInstance: "old", State: "failed", Code: &code, CreatedAt: now, UpdatedAt: now, Deadline: now.Add(time.Minute), SettledAt: &now}
			require.NoError(t, db.Create(&busy).Error, "terminal host.busy is admitted by the forward CHECK")
			busy.RunID, busy.State, busy.Code, busy.SettledAt = "active", "reserved", nil, nil
			require.NoError(t, db.Create(&busy).Error)
			busy.RunID = "duplicate"
			require.Error(t, db.Create(&busy).Error, "active environment uniqueness survives the rebuild")
			require.Equal(t, [][]string{{"ok"}}, sqlRows(t, actual, "PRAGMA integrity_check"))
			require.Empty(t, sqlRows(t, actual, "PRAGMA foreign_key_check"))
		})
	}
}
