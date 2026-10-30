// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original schema-013 upgrade preservation and reopen proof for both roles.

package schema

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	containerDTO "github.com/florianfinn/docklet-next/types/container"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/backend/resources"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestContainerHost014UpgradeBothRoles(t *testing.T) {
	for _, role := range []string{"hub", "agent"} {
		t.Run(role, func(t *testing.T) {
			initialize := database.Initialize
			if role == "hub" {
				initialize = database.InitializeHub
			}
			raw := sqlitestore.URL(filepath.Join(t.TempDir(), role+".db"))
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
			for _, state := range []string{"pending", "unknown", "success"} {
				_, err = pool.ExecContext(t.Context(), `INSERT INTO runtime_receipts(run_id,kind,store_generation,manager_id,registration_id,environment_id,executor_generation,request_digest,caller_id,operation_id,target_kind,target_id,gate_scope,gate_key,admitted_at,dispatch_state,executor_state,unknown_reason,executor_settled_at,settled_at,cancel_requested,release_acknowledged,final_write_at) VALUES(?,'receipt','store','manager','registration','0','store','digest','operator','start-container','container',?,?,'start',CURRENT_TIMESTAMP,'pending',?,?,?, ?,0,0,CURRENT_TIMESTAMP)`, state, state, "container:"+state, state, nullable014(state == "unknown", "lost"), nullable014(state == "success", time.Now()), nullable014(state == "success", time.Now()))
				require.NoError(t, err)
				_, err = pool.ExecContext(t.Context(), `INSERT INTO runtime_key_bindings(environment_id,idempotency_key,run_id,caller_id,request_digest,first_seen_at) VALUES('0',?,?,'operator','digest',CURRENT_TIMESTAMP)`, "key-"+state, state)
				require.NoError(t, err)
			}
			require.NoError(t, pool.Close())
			db, err := initialize(t.Context(), raw, database.MigrationOptions{})
			require.NoError(t, err)
			require.NoError(t, db.Close())
			db, err = initialize(t.Context(), raw, database.MigrationOptions{})
			require.NoError(t, err)
			defer db.Close()
			var n int64
			require.NoError(t, db.Table("runtime_key_bindings").Count(&n).Error)
			require.EqualValues(t, 3, n)
			require.NoError(t, db.Table("runtime_receipts").Count(&n).Error)
			require.EqualValues(t, 3, n)
			for _, state := range []string{"pending", "unknown", "success"} {
				var got struct {
					ExecutorState string
					UnknownReason *string
					SettledAt     *time.Time
					Result        *string
				}
				require.NoError(t, db.Table("runtime_receipts").Where("run_id=?", state).Take(&got).Error)
				require.Equal(t, state, got.ExecutorState)
				require.Equal(t, state == "unknown", got.UnknownReason != nil)
				require.Equal(t, state == "success", got.SettledAt != nil)
				require.Nil(t, got.Result)
			}
			for _, op := range []struct{ operation, kind string }{{"recreate-container", "container"}, {"redeploy-container", "container"}, {"stop-project", "stack"}} {
				require.NoError(t, db.Exec(`INSERT INTO runtime_receipts(run_id,kind,store_generation,manager_id,registration_id,environment_id,executor_generation,request_digest,caller_id,operation_id,target_kind,target_id,gate_scope,gate_key,admitted_at,dispatch_state,executor_state,cancel_requested,release_acknowledged,final_write_at) VALUES(?,'receipt','store','manager','registration','0','store','digest','operator',?,?,?,?,'operation',CURRENT_TIMESTAMP,'pending','pending',0,0,CURRENT_TIMESTAMP)`, op.operation, op.operation, op.kind, op.operation, op.kind+":"+op.operation).Error)
				wrong := "container"
				if op.kind == "container" {
					wrong = "stack"
				}
				require.Error(t, db.Table("runtime_receipts").Where("run_id=?", op.operation).Update("target_kind", wrong).Error)
			}
			for _, index := range []string{"idx_runtime_receipts_environment_run", "idx_runtime_receipts_active_scope", "idx_runtime_receipts_active_parent", "idx_runtime_receipts_retention", "idx_runtime_key_bindings_run"} {
				require.NoError(t, db.Raw("SELECT count(*) FROM sqlite_master WHERE type='index' AND name=?", index).Scan(&n).Error)
				require.EqualValues(t, 1, n, index)
			}
			admit014Operations(t, db)
			var foreign []map[string]any
			require.NoError(t, db.Raw("PRAGMA foreign_key_check").Scan(&foreign).Error)
			require.Empty(t, foreign)
		})
	}
}
func nullable014(ok bool, value any) any {
	if ok {
		return value
	}
	return nil
}

type resolver014 struct{}

func (resolver014) ResolveRuntimeTarget(_ context.Context, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
	return activity.RuntimeResolvedTarget{Target: target}, nil
}
func admit014Operations(t *testing.T, db *database.DB) {
	t.Helper()
	service := activity.NewRuntimeService(activity.NewActivityService(db, nil), resolver014{})
	generation, err := service.InitializeGeneration(t.Context())
	require.NoError(t, err)
	store := activity.RuntimeStoreBinding{StoreGeneration: generation, ExecutorGeneration: generation, ManagerID: "example-manager", RegistrationID: "example-registration", EnvironmentID: "0"}
	_, err = activity.NewRuntimeHTTPService(service, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return store, nil }, PrepareTarget: resolver014{}.ResolveRuntimeTargetWithEnvironment, GuardContainer: func(context.Context, string, string) error { return nil }})
	require.NoError(t, err)
	for i, operation := range []activity.RuntimeOperation{activity.RuntimeRecreateContainer, activity.RuntimeRedeployContainer, activity.RuntimeStopProject} {
		target := activity.RuntimeTarget{Kind: "container", ID: strings.Repeat(string(rune('a'+i)), 64)}
		if operation == activity.RuntimeStopProject {
			target = activity.RuntimeTarget{Kind: "stack", ID: "example-stack"}
		}
		command := activity.RuntimeCommand{Store: store, Key: uuid.NewString(), Actor: userDTO.Actor{ID: "example-owner", Username: "Operator"}, Operation: operation, Target: target}
		if operation == activity.RuntimeRecreateContainer {
			command.Options.Recreate = &containerDTO.Edit{}
		}
		admitted, err := service.Admit(t.Context(), command)
		require.NoError(t, err)
		var row activity.RuntimeReceipt
		require.NoError(t, db.First(&row, "run_id=?", admitted.ActivityID).Error)
		require.Equal(t, new(string(operation)), row.OperationID)
		require.Equal(t, new(target.Kind), row.TargetKind)
	}
}
func (r resolver014) ResolveRuntimeTargetWithEnvironment(ctx context.Context, _ string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
	return r.ResolveRuntimeTarget(ctx, target)
}
