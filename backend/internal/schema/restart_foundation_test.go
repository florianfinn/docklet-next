// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original durable restart migration refusal and reopen evidence.

package schema

import (
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/backend/resources"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"io/fs"
	"path/filepath"
	"testing"
	"time"
)

func TestRestartFoundationFreshUpgradeReopen(t *testing.T) {
	for _, upgrade := range []bool{false, true} {
		for _, hub := range []bool{false, true} {
			t.Run(filepath.Base(t.TempDir()), func(t *testing.T) {
				raw := sqlitestore.URL(filepath.Join(t.TempDir(), "restart.db"))
				if upgrade {
					path, err := sqlitestore.Path(raw)
					require.NoError(t, err)
					pool, err := sqlitestore.Open(t.Context(), path, false, false)
					require.NoError(t, err)
					files, err := fs.Sub(resources.FS, "migrations/sqlite")
					require.NoError(t, err)
					provider, err := goose.NewProvider(goose.DialectSQLite3, pool, files)
					require.NoError(t, err)
					_, err = provider.UpTo(t.Context(), 13)
					require.NoError(t, err)
					_, err = pool.ExecContext(t.Context(), `INSERT INTO environments(id,name,api_url) VALUES('example-host','Example','https://agent.example.com')`)
					require.NoError(t, err)
					require.NoError(t, pool.Close())
				}
				initialize := database.Initialize
				if hub {
					initialize = database.InitializeHub
				}
				db, err := initialize(t.Context(), raw, database.MigrationOptions{})
				require.NoError(t, err)
				now := time.Now().UTC()
				for i, updates := range []map[string]any{
					{"state": "failed", "settled_at": now},
					{"state": "unknown", "settled_at": now},
					{"code": "agent.restart-timeout"},
					{"state": "unknown", "settled_at": now, "code": "agent.restart-timeout"},
					{"state": "failed", "settled_at": now, "code": "agent.restart-unconfirmed"},
					{"state": "failed", "settled_at": now, "code": "invalid"},
					{"acknowledged_at": now},
				} {
					row := environment.AgentRestartRequest{RunID: "example-run", EnvironmentID: "example-host", RegistrationID: "example-registration", OldInstance: "example-old", State: "reserved", CreatedAt: now, UpdatedAt: now, Deadline: now.Add(time.Minute)}
					require.NoError(t, db.Create(&row).Error)
					require.Error(t, db.Model(&row).Updates(updates).Error, "CHECK case %d", i)
					require.NoError(t, db.Delete(&row).Error)
				}
				for _, code := range []string{"agent.restart-superseded", "agent.restart-timeout", "agent.restart-unsupported", "agent.restart-interrupted", "host.unreachable"} {
					row := environment.AgentRestartRequest{RunID: code, EnvironmentID: code, RegistrationID: "example-registration", OldInstance: "example-old", State: "failed", Code: &code, CreatedAt: now, UpdatedAt: now, Deadline: now.Add(time.Minute), SettledAt: &now, SentAt: &now, AcknowledgedAt: &now}
					require.NoError(t, db.Create(&row).Error)
				}
				require.NoError(t, db.Close())
				db, err = initialize(t.Context(), raw, database.MigrationOptions{})
				require.NoError(t, err)
				defer db.Close()
				var rows []environment.AgentRestartRequest
				require.NoError(t, db.Find(&rows).Error)
				require.Len(t, rows, 5)
				for _, row := range rows {
					require.Equal(t, row.RunID, *row.Code)
					require.NotNil(t, row.AcknowledgedAt)
					require.Error(t, db.Model(&row).Update("code", nil).Error)
				}
				if upgrade {
					var n int64
					require.NoError(t, db.Table("environments").Where("id=?", "example-host").Count(&n).Error)
					require.EqualValues(t, 1, n)
				}
			})
		}
	}
}
