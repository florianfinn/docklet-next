// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Restore repair proof with real paired archives and migrated SQLite services.

package backup

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type restoreArchiveSnapshot struct {
	Receipts    []activity.RuntimeReceipt
	Bindings    []activity.RuntimeKeyBinding
	Generations []activity.RuntimeStoreGeneration
	Activities  []activity.Activity
	Messages    []activity.ActivityMessage
}

func restoreArchiveRead(t *testing.T, db *gorm.DB) restoreArchiveSnapshot {
	t.Helper()
	var s restoreArchiveSnapshot
	require.NoError(t, db.Order("run_id").Find(&s.Receipts).Error)
	require.NoError(t, db.Order("environment_id,idempotency_key").Find(&s.Bindings).Error)
	require.NoError(t, db.Order("id").Find(&s.Generations).Error)
	require.NoError(t, db.Order("id").Find(&s.Activities).Error)
	require.NoError(t, db.Order("activity_id,id").Find(&s.Messages).Error)
	return s
}

type restoreArchiveResolver struct{}

func (restoreArchiveResolver) ResolveRuntimeTarget(_ context.Context, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
	return activity.RuntimeResolvedTarget{Target: target, ComposeManaged: target.ParentProjectID != nil}, nil
}

type restoreArchiveFixture struct {
	db         *database.DB
	cfg        *config.Config
	activities *activity.ActivityService
	runtime    *activity.RuntimeService
	binding    activity.RuntimeStoreBinding
	held       activity.RuntimeCommand
	heldID     string
	legacy     []string
}

func restoreArchiveNew(t *testing.T) *restoreArchiveFixture {
	t.Helper()
	db, cfg := archiveFixture(t)
	a := activity.NewActivityService(db, nil)
	r := activity.NewRuntimeService(a, restoreArchiveResolver{})
	generation, err := r.InitializeGeneration(t.Context())
	require.NoError(t, err)
	return &restoreArchiveFixture{db: db, cfg: cfg, activities: a, runtime: r, binding: activity.RuntimeStoreBinding{
		StoreGeneration: generation, ExecutorGeneration: generation, ManagerID: "example-manager", RegistrationID: "example-registration", EnvironmentID: "0"}}
}

func (f *restoreArchiveFixture) command(name string) activity.RuntimeCommand {
	return activity.RuntimeCommand{Store: f.binding, Key: uuid.NewString(), Actor: usertypes.Actor{ID: "example-owner", Username: "Original operator"},
		Operation: activity.RuntimeDeployProject, Target: activity.RuntimeTarget{Kind: "stack", ID: "example-" + name}}
}

func (f *restoreArchiveFixture) admit(t *testing.T, c activity.RuntimeCommand) string {
	t.Helper()
	result, err := f.runtime.Admit(t.Context(), c)
	require.NoError(t, err)
	require.Equal(t, activitytypes.CodeRunStarted, result.Code)
	return result.ActivityID
}

func (f *restoreArchiveFixture) legacyRow(t *testing.T, scope string, status activitytypes.Status) string {
	t.Helper()
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	id := uuid.NewString()
	key := ""
	if scope != "" {
		key = "example-key"
	}
	row := activity.Activity{BaseModel: database.BaseModel{ID: id, CreatedAt: now, UpdatedAt: new(now)}, EnvironmentID: "0", Type: activitytypes.TypeResourceAction,
		Status: status, StartedAt: now, GateScope: scope, GateKey: key, StartedByUserID: new("example-owner"), StartedByUsername: new("Original operator"),
		Metadata: database.JSON{"example": "retained"}, AffectedServices: database.JSON{}, Output: activity.OutputLines{{Text: "Retained example output"}}}
	require.NoError(t, f.db.Create(&row).Error)
	return id
}

func (f *restoreArchiveFixture) seed(t *testing.T) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	for i, phase := range []string{"pending", "accepted", "executing", "dispatching", "unknown", "terminal", "disposed"} {
		c := f.command(phase)
		if i%2 == 0 {
			c.Operation = activity.RuntimeStartContainer
			c.Target = activity.RuntimeTarget{Kind: "container", ID: strings.Repeat(fmt.Sprintf("%x", i+1), 64), ParentProjectID: new("example-parent-" + phase)}
		}
		if phase == "dispatching" || phase == "unknown" || phase == "disposed" {
			c.Store.EnvironmentID = "example-remote"
			c.Store.ExecutorGeneration = uuid.NewString()
		}
		id := f.admit(t, c)
		if phase == "pending" {
			f.held = c
			f.heldID = id
		}
		changes := map[string]any{}
		switch phase {
		case "accepted", "executing":
			changes["executor_state"] = phase
		case "dispatching":
			changes["dispatch_state"] = "dispatching"
			changes["dispatched_at"] = now
		case "unknown":
			changes["executor_state"] = "unknown"
			changes["unknown_reason"] = "executor-unavailable"
			changes["dispatch_state"] = "dispatching"
			changes["dispatched_at"] = now
		case "terminal":
			changes["executor_state"] = "success"
			changes["executor_settled_at"] = now
			changes["settled_at"] = now
		case "disposed":
			changes["executor_state"] = "unknown"
			changes["unknown_reason"] = "executor-unavailable"
			changes["hub_disposition"] = "released"
			changes["hub_disposition_by"] = "example-admin"
			changes["hub_disposition_reason"] = "Example acknowledged release"
			changes["hub_disposition_at"] = now
			changes["release_acknowledged"] = true
			changes["settled_at"] = now
		}
		if i%2 == 1 {
			changes["cancel_requested"] = true
			changes["cancel_requested_at"] = now
		}
		if len(changes) > 0 {
			require.NoError(t, f.db.Model(&activity.RuntimeReceipt{}).Where("run_id = ?", id).Updates(changes).Error)
		}
		scope, key := "", ""
		if i%2 == 1 {
			scope = "foreign:copied-" + phase
			key = "foreign-key"
		}
		status := activitytypes.StatusRunning
		if i%2 == 0 {
			status = activitytypes.StatusQueued
		}
		require.NoError(t, f.db.Model(&activity.Activity{}).Where("id = ?", id).UpdateColumns(map[string]any{"gate_scope": scope, "gate_key": key, "status": status, "output": activity.OutputLines{{Text: "Retained runtime output"}}, "output_truncated": true}).Error)
	}
	for _, scope := range []string{"", "stack:example-no-receipt", "ordinary:example", "system-backup", "coordinator:example", "job:example"} {
		id := f.legacyRow(t, scope, activitytypes.StatusRunning)
		f.legacy = append(f.legacy, id)
		if strings.HasPrefix(scope, "job:") {
			require.NoError(t, f.db.Model(&activity.Activity{}).Where("id = ?", id).UpdateColumn("type", activitytypes.TypeJobRun).Error)
		}
	}
	wrong := f.command("wrong-environment")
	wrong.Store.EnvironmentID = "example-other-environment"
	id := f.admit(t, wrong)
	f.legacy = append(f.legacy, id)
	require.NoError(t, f.db.Model(&activity.Activity{}).Where("id = ?", id).UpdateColumn("environment_id", "0").Error)
	tomb := f.legacyRow(t, "stack:example-tombstone", activitytypes.StatusQueued)
	f.legacy = append(f.legacy, tomb)
	require.NoError(t, f.db.Create(&activity.RuntimeReceipt{RunID: tomb, Kind: "not_received", StoreGeneration: f.binding.StoreGeneration, ExecutorGeneration: f.binding.ExecutorGeneration, EnvironmentID: "0", ManagerID: f.binding.ManagerID, RegistrationID: f.binding.RegistrationID, RequestDigest: "example-tombstone", SettledAt: new(now), FinalWriteAt: now}).Error)
	for _, status := range []activitytypes.Status{activitytypes.StatusSuccess, activitytypes.StatusFailed, activitytypes.StatusCancelled} {
		f.legacyRow(t, "terminal:"+string(status), status)
	}
	require.NoError(t, f.db.Exec("INSERT INTO users(id,created_at,username,password_hash) VALUES('example-user',1,'example','hash')").Error)
	require.NoError(t, f.db.Exec("INSERT INTO user_sessions(id,created_at,user_id,refresh_token_hash,last_used_at,expires_at) VALUES('example-session',1,'example-user','hash',1,9999999999999999)").Error)
	sidecar, err := sqlitestore.Open(t.Context(), sqlitestore.FrancisPath(f.db.Path()), false, false)
	require.NoError(t, err)
	_, err = sidecar.ExecContext(t.Context(), "INSERT INTO arcane_francis_hosts(host_id,host_address) VALUES('example-owner','example.invalid:3551')")
	require.NoError(t, err)
	require.NoError(t, sidecar.Close())
}

func restoreArchiveDeltas(t *testing.T, before, after restoreArchiveSnapshot, legacy []string, operationID string) {
	t.Helper()
	expected := before
	expected.Activities = append([]activity.Activity(nil), before.Activities...)
	allowed := map[string]bool{}
	for _, id := range legacy {
		allowed[id] = true
	}
	for i, row := range expected.Activities {
		if !allowed[row.ID] {
			continue
		}
		var actual activity.Activity
		for _, a := range after.Activities {
			if a.ID == row.ID {
				actual = a
			}
		}
		require.Equal(t, activitytypes.StatusFailed, actual.Status, "legacy control %s must repair", row.GateScope)
		require.Equal(t, new("backup.interrupted"), actual.Error)
		require.Equal(t, "activity.failed", actual.MessageCode)
		require.NotNil(t, actual.EndedAt)
		require.NotNil(t, actual.UpdatedAt)
		require.NotNil(t, actual.DurationMs)
		require.Equal(t, actual.EndedAt, actual.UpdatedAt)
		require.Equal(t, max(int64(0), (actual.EndedAt.UnixMicro()-row.StartedAt.UnixMicro())/1000), *actual.DurationMs)
		row.Status = actual.Status
		row.Error = actual.Error
		row.MessageCode = actual.MessageCode
		row.EndedAt = actual.EndedAt
		row.UpdatedAt = actual.UpdatedAt
		row.DurationMs = actual.DurationMs
		expected.Activities[i] = row
	}
	filtered := make([]activity.Activity, 0, len(after.Activities))
	for _, row := range after.Activities {
		if row.ID != operationID {
			filtered = append(filtered, row)
		}
	}
	after.Activities = filtered
	messages := make([]activity.ActivityMessage, 0, len(after.Messages))
	for _, row := range after.Messages {
		if row.ActivityID != operationID {
			messages = append(messages, row)
		}
	}
	after.Messages = messages
	assert.Equal(t, expected, after, "only named legacy and current restore operation deltas are allowed")
}

func restoreArchivePeer(t *testing.T, f *restoreArchiveFixture) *activity.RuntimeService {
	t.Helper()
	// Open only after replacement; the original DB remains the sole lifetime owner.
	pool, err := sqlitestore.Open(t.Context(), f.db.Path(), true, false)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	db, err := gorm.Open(sqlitestore.Dialector(pool), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	return activity.NewRuntimeService(activity.NewActivityService(&database.DB{DB: db}, nil), restoreArchiveResolver{})
}

func restoreArchiveHeld(t *testing.T, f *restoreArchiveFixture) {
	t.Helper()
	peer := restoreArchivePeer(t, f)
	for _, parent := range []bool{false, true} {
		c := f.held
		c.Key = uuid.NewString()
		c.Operation = activity.RuntimeStopContainer
		if parent {
			c.Operation = activity.RuntimeDownProject
			c.Target = activity.RuntimeTarget{Kind: "stack", ID: *f.held.Target.ParentProjectID}
		}
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		result, err := peer.Admit(ctx, c)
		cancel()
		var conflict *activity.GateConflict
		require.ErrorAs(t, err, &conflict)
		require.Equal(t, f.heldID, conflict.RunID)
		require.Empty(t, result.ActivityID)
	}
}

func restoreArchiveMetadata(t *testing.T, f *restoreArchiveFixture) *backuptypes.SystemBackupRun {
	t.Helper()
	name, size, err := fixtureArchive(t, f.db, f.cfg, "example-receipts", 2<<30)
	require.NoError(t, err)
	return &backuptypes.SystemBackupRun{ID: "example-receipts", Status: backuptypes.SystemBackupStatusSucceeded,
		ArchiveName: name, Size: size, MigrationVersion: mustNewestMigration(), ArchiveVersion: archiveVersion}
}

func restoreArchiveInstall(t *testing.T, source *restoreArchiveFixture, record *backuptypes.SystemBackupRun) *restoreArchiveFixture {
	t.Helper()
	target := restoreArchiveNew(t)
	target.cfg.BackupDir = source.cfg.BackupDir
	target.binding = source.binding
	target.held = source.held
	target.heldID = source.heldID
	target.legacy = source.legacy
	require.NoError(t, target.db.SuspendOrdinaryConnections())
	require.NoError(t, checkpointStoppedPair(t.Context(), target.db.Path()))
	marker, err := preparePair(t.Context(), target.cfg, target.db.Path(), record.ID, "")
	require.NoError(t, err)
	require.NoError(t, installPair(t.Context(), restoreStateDir(target.db.Path()), marker))
	require.NoError(t, target.db.ResumeOrdinaryConnections())
	return target
}
