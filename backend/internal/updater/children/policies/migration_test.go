// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Forward 008-to-009 keeps existing lifecycle data and enforces policy constraints.

package policies

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

func TestPolicyForwardMigrationFrom008AndReopen(t *testing.T) {
	raw := sqlitestore.URL(filepath.Join(t.TempDir(), "forward.db"))
	path, err := sqlitestore.Path(raw)
	require.NoError(t, err)
	pool, err := sqlitestore.Open(t.Context(), path, false, false)
	require.NoError(t, err)
	files, err := fs.Sub(resources.FS, "migrations/sqlite")
	require.NoError(t, err)
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, files)
	require.NoError(t, err)
	_, err = provider.UpTo(t.Context(), 8)
	require.NoError(t, err)
	_, err = pool.ExecContext(t.Context(), `INSERT INTO restore_points (id,target_id,target_identity,registration_id,source_run_id,created_at,failed_attempt,source_state,revision,payload,available) VALUES ('preserved-point','example-stack','identity','registration','source',CURRENT_TIMESTAMP,0,'settled','revision','synthetic-encrypted-payload',1)`)
	require.NoError(t, err)
	require.NoError(t, pool.Close())
	db, err := database.InitializeHub(t.Context(), raw, database.MigrationOptions{})
	require.NoError(t, err)
	p := Policy{ID: "policy", EnvironmentID: "0", RegistrationID: "registration", TargetID: "example-stack", TargetIdentity: "identity", Mode: "off", Revision: 1, Candidates: "[]"}
	require.NoError(t, db.Create(&p).Error)
	for _, query := range []string{
		"UPDATE stack_update_policies SET mode='container'", "UPDATE stack_update_policies SET revision=0",
	} {
		require.ErrorContains(t, db.Exec(query).Error, "CHECK constraint failed")
	}
	require.NoError(t, db.Close())
	db, err = database.InitializeHub(t.Context(), raw, database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	var payload string
	require.NoError(t, db.Table("restore_points").Select("payload").Where("id = ?", "preserved-point").Scan(&payload).Error)
	require.Equal(t, "synthetic-encrypted-payload", payload)
	var saved Policy
	require.NoError(t, db.First(&saved, "id = ?", "policy").Error)
	require.Equal(t, "off", saved.Mode)
	var integrity string
	require.NoError(t, db.Raw("PRAGMA integrity_check").Scan(&integrity).Error)
	require.Equal(t, "ok", integrity)
	var foreign []map[string]any
	require.NoError(t, db.Raw("PRAGMA foreign_key_check").Scan(&foreign).Error)
	require.Empty(t, foreign)
}
