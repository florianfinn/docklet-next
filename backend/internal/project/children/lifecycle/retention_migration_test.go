// Source: docklet-next/docs/design/images-updates.md
// Revision: 727498b22e03475aa5b4e5325727ff182e8cebdd
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Retention metadata survives history deletion and reopen; M21 seeds it on the release baseline.

package lifecycle_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
)

func TestRetentionMetadataSurvivesHistoryDeletionAndReopen(t *testing.T) {
	raw := sqlitestore.URL(filepath.Join(t.TempDir(), "retention.db"))
	db, err := database.Initialize(t.Context(), raw, database.MigrationOptions{})
	require.NoError(t, err)
	restoredAt := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	require.NoError(t, db.Exec(`INSERT INTO restore_points (id,target_id,target_identity,registration_id,source_run_id,created_at,source_state,revision,payload,available,last_restored_at) VALUES ('selected-point','example-stack','identity','registration','source',CURRENT_TIMESTAMP,'settled','revision','synthetic-encrypted-payload',1,?)`, restoredAt).Error)
	require.NoError(t, db.Exec(`INSERT INTO restore_points (id,target_id,target_identity,registration_id,source_run_id,created_at,source_state,revision,payload,available) VALUES ('unused-point','example-stack','identity','registration','source',CURRENT_TIMESTAMP,'settled','revision','synthetic-encrypted-payload',1)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO runtime_receipts (run_id,kind,store_generation,manager_id,registration_id,environment_id,executor_generation,request_digest,caller_id,operation_id,target_kind,target_id,gate_scope,gate_key,admitted_at,dispatch_state,executor_state,cancel_requested,release_acknowledged,final_write_at) VALUES ('restore-run','receipt','store','manager','registration','0','store','digest','operator','restore-project','stack','example-stack','stack:example-stack','restore','2026-10-25 12:00:00','pending','pending',0,0,CURRENT_TIMESTAMP)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO runtime_image_intents (run_id,payload) VALUES ('restore-run','{"request":{"pointId":"selected-point"}}')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO restore_points (id,target_id,target_identity,registration_id,source_run_id,created_at,source_state,revision,payload,available,failed_attempt) VALUES ('failed-point','example-stack','identity','registration','failed-run',CURRENT_TIMESTAMP,'partial-source','revision','synthetic-failed-payload',1,1)`).Error)
	require.NoError(t, db.Close())
	db, err = database.Initialize(t.Context(), raw, database.MigrationOptions{})
	require.NoError(t, err)
	var selected, unused life.Point
	require.NoError(t, db.First(&selected, "id = ?", "selected-point").Error)
	require.NoError(t, db.First(&unused, "id = ?", "unused-point").Error)
	require.NotNil(t, selected.LastRestoredAt)
	require.Equal(t, restoredAt, selected.LastRestoredAt.UTC())
	require.Nil(t, unused.LastRestoredAt)
	require.False(t, selected.FailedAttempt)
	var failed life.Point
	require.NoError(t, db.First(&failed, "id = ?", "failed-point").Error)
	require.True(t, failed.FailedAttempt)
	require.ErrorContains(t, db.Exec("UPDATE restore_points SET failed_attempt = 2 WHERE id = ?", "failed-point").Error, "CHECK constraint failed")
	var live int64
	require.NoError(t, db.Table("runtime_receipts").Where("run_id = ?", "restore-run").Count(&live).Error)
	require.EqualValues(t, 1, live)
	var foreign []map[string]any
	require.NoError(t, db.Raw("PRAGMA foreign_key_check").Scan(&foreign).Error)
	require.Empty(t, foreign)
	var integrity string
	require.NoError(t, db.Raw("PRAGMA integrity_check").Scan(&integrity).Error)
	require.Equal(t, "ok", integrity)

	require.Equal(t, "synthetic-failed-payload", failed.Payload)
	require.Equal(t, "synthetic-encrypted-payload", selected.Payload)
	require.True(t, selected.Available)
	require.NoError(t, db.Exec("DELETE FROM runtime_image_intents").Error)
	require.NoError(t, db.Exec("DELETE FROM runtime_receipts").Error)
	require.NoError(t, db.Close())
	db, err = database.Initialize(t.Context(), raw, database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.First(&selected, "id = ?", "selected-point").Error)
	require.NotNil(t, selected.LastRestoredAt, "retention is independent of receipt history")
	require.Equal(t, "synthetic-encrypted-payload", selected.Payload)
	require.NoError(t, db.First(&failed, "id = ?", "failed-point").Error)
	require.True(t, failed.FailedAttempt)
}
