// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Migration 006 to 007 with live receipt/key rows, encrypted snapshot reopen and key refusal.

package lifecycle_test

import (
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/backend/resources"
	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

func TestLifecycleForwardMigrationPreservesLiveReceiptsAndReopens(t *testing.T) {
	raw := sqlitestore.URL(filepath.Join(t.TempDir(), "lifecycle.db"))
	path, err := sqlitestore.Path(raw)
	require.NoError(t, err)
	pool, err := sqlitestore.Open(t.Context(), path, false, false)
	require.NoError(t, err)
	files, err := fs.Sub(resources.FS, "migrations/sqlite")
	require.NoError(t, err)
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, files)
	require.NoError(t, err)
	_, err = provider.UpTo(t.Context(), 6)
	require.NoError(t, err)
	run, key := uuid.NewString(), uuid.NewString()
	_, err = pool.ExecContext(t.Context(), `INSERT INTO runtime_receipts (run_id,kind,store_generation,manager_id,registration_id,environment_id,executor_generation,request_digest,caller_id,operation_id,target_kind,target_id,gate_scope,gate_key,admitted_at,dispatch_state,executor_state,cancel_requested,release_acknowledged,final_write_at) VALUES (?,'receipt','store','manager','registration','0','store','digest','operator','deploy-project','stack','example-stack','stack:example-stack','deploy',CURRENT_TIMESTAMP,'pending','pending',0,0,CURRENT_TIMESTAMP)`, run)
	require.NoError(t, err)
	_, err = pool.ExecContext(t.Context(), `INSERT INTO runtime_key_bindings (environment_id,idempotency_key,run_id,caller_id,request_digest,first_seen_at) VALUES ('0',?,?,'operator','digest',CURRENT_TIMESTAMP)`, key, run)
	require.NoError(t, err)
	require.NoError(t, pool.Close())
	db, err := database.Initialize(t.Context(), raw, database.MigrationOptions{})
	require.NoError(t, err)
	var retained struct {
		OperationID string
		SettledAt   *time.Time
	}
	require.NoError(t, db.Table("runtime_receipts").Where("run_id = ?", run).Take(&retained).Error)
	require.Equal(t, "deploy-project", retained.OperationID)
	require.Nil(t, retained.SettledAt)
	var n int64
	require.NoError(t, db.Table("runtime_key_bindings").Where("idempotency_key = ? AND run_id = ?", key, run).Count(&n).Error)
	require.EqualValues(t, 1, n)
	require.NoError(t, db.Table("runtime_receipts").Where("run_id = ?", run).Update("operation_id", "update-project").Error)
	require.Error(t, db.Table("runtime_receipts").Where("run_id = ?", run).Update("operation_id", "update-container").Error)
	require.NoError(t, db.Close())
	db, err = database.Initialize(t.Context(), raw, database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.Table("runtime_key_bindings").Where("idempotency_key = ?", key).Count(&n).Error)
	require.EqualValues(t, 1, n)
	var foreign []map[string]any
	require.NoError(t, db.Raw("PRAGMA foreign_key_check").Scan(&foreign).Error)
	require.Empty(t, foreign)
}
func TestLifecycleEncryptedSnapshotsPersistAndWeakKeysCannotReplace(t *testing.T) {
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: uuid.NewString()})
	raw := sqlitestore.URL(filepath.Join(t.TempDir(), "snapshot.db"))
	db, err := database.Initialize(t.Context(), raw, database.MigrationOptions{})
	require.NoError(t, err)
	snapshot := &life.Snapshot{TargetID: "example-stack", TargetIdentity: life.Hash("identity"), RegistrationID: "registration", RunID: uuid.NewString(), SavedRevision: life.Hash("files"), Files: []life.File{{Path: "compose.yaml", Present: true, Content: []byte("services: {}\n")}, {Path: ".env", Present: true, Content: []byte("SYNTHETIC_INPUT=retained-input\n")}}, Model: &compose.Project{Name: "example-stack", WorkingDir: "/example/stack", ComposeFiles: []string{"/example/stack/compose.yaml"}, Environment: compose.Mapping{"SYNTHETIC_INPUT": "retained-input"}, Services: compose.Services{"app": {Name: "app", Image: "registry.example.com/app:1.0.0"}}}, Services: map[string]life.ServiceImage{"app": {Reference: "registry.example.com/app:1.0.0", ImageID: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Present: true}}}
	store := life.Store{DB: db}
	require.NoError(t, store.SaveBaseline(t.Context(), snapshot))
	point, err := store.Prepare(t.Context(), snapshot, uuid.NewString(), "", "settled")
	require.NoError(t, err)
	require.NoError(t, store.Available(t.Context(), point.ID))
	require.NotContains(t, point.Payload, "SYNTHETIC_INPUT")
	require.NotContains(t, point.Payload, "retained-input")
	require.NoError(t, db.Close())
	db, err = database.Initialize(t.Context(), raw, database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	store.DB = db
	reopened, saved, err := store.Point(t.Context(), point.ID)
	require.NoError(t, err)
	require.True(t, reopened.Available)
	require.Equal(t, snapshot.Files, saved.Files)
	require.Equal(t, snapshot.Services, saved.Services)
	require.Equal(t, snapshot.Model.WorkingDir, saved.Model.WorkingDir)
	require.Equal(t, snapshot.Model.ComposeFiles, saved.Model.ComposeFiles)
	require.Equal(t, snapshot.Model.Environment, saved.Model.Environment)
	require.Equal(t, "app", saved.Model.Services["app"].Name)
	_, revision, err := store.Baseline(t.Context(), snapshot)
	require.NoError(t, err)
	require.Equal(t, life.Hash(snapshot), revision)
	var before life.Baseline
	require.NoError(t, db.First(&before, "target_id = ?", snapshot.TargetID).Error)
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: ""})
	t.Cleanup(func() {
		crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: uuid.NewString()})
	})
	assert.Error(t, store.SaveBaseline(t.Context(), snapshot))
	_, err = store.Prepare(t.Context(), snapshot, uuid.NewString(), "", "partial-source")
	assert.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&life.Point{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	var baseline life.Baseline
	require.NoError(t, db.First(&baseline, "target_id = ?", snapshot.TargetID).Error)
	require.Equal(t, before.Payload, baseline.Payload)
	require.True(t, baseline.Available)
	require.Equal(t, revision, baseline.Revision)
}
