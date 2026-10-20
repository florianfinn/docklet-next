// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Activated paired restore, session revocation and receiver readiness proof.

package backup

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/auth"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type restoreActivationExecutor struct{ calls atomic.Int32 }

func (e *restoreActivationExecutor) ExecuteRuntime(context.Context, activity.RuntimeCommand, io.Writer) error {
	e.calls.Add(1)
	return nil
}

type restoreActivationTransport struct{ calls atomic.Int32 }

func (r *restoreActivationTransport) Execute(context.Context, activitytypes.RuntimeExecutionEnvelope) (activitytypes.RuntimeExecutionOutcome, error) {
	r.calls.Add(1)
	return activitytypes.RuntimeExecutionOutcome{}, activity.ErrRuntimeTransport
}
func (r *restoreActivationTransport) Query(context.Context, activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error) {
	return activitytypes.RuntimeExecutionOutcome{}, activity.ErrRuntimeTransport
}
func (r *restoreActivationTransport) Cancel(context.Context, activitytypes.RuntimeCancellation) (activitytypes.RuntimeExecutionOutcome, error) {
	return activitytypes.RuntimeExecutionOutcome{}, activity.ErrRuntimeTransport
}

func TestRuntimeActivationActualPairedRestore(t *testing.T) {
	for _, fault := range []string{"none", "generation-write"} {
		t.Run(fault, func(t *testing.T) {
			source := restoreArchiveNew(t)
			source.binding.RegistrationID = source.binding.ManagerID
			require.NoError(t, source.db.Exec("INSERT INTO settings(key,value) VALUES('instanceId',?)", source.binding.ManagerID).Error)
			local := source.command("restored-local")
			localID := source.admit(t, local)
			remote := source.command("restored-remote")
			remote.Store.EnvironmentID, remote.Store.RegistrationID = "example-remote", "example-remote"
			remote.Store.ExecutorGeneration = uuid.NewString()
			remoteID := source.admit(t, remote)
			require.NoError(t, source.db.Model(&activity.RuntimeReceipt{}).Where("run_id = ?", remoteID).Updates(map[string]any{"dispatch_state": "dispatching", "dispatched_at": time.Now().UTC()}).Error)
			require.NoError(t, source.db.Exec("INSERT INTO users(id,created_at,username,password_hash) VALUES('example-user',1,'example','hash')").Error)
			oldSession, _, err := session.NewSessionService(source.db).CreateSession(t.Context(), "example-user", time.Now().Add(time.Hour), auth.SessionMeta{})
			require.NoError(t, err)
			require.NoError(t, session.ValidateActive(oldSession))
			if fault != "none" {
				require.NoError(t, source.db.Exec("CREATE TRIGGER fixture_activation_generation BEFORE UPDATE ON runtime_store_generation BEGIN SELECT RAISE(ABORT,'synthetic generation refusal'); END").Error)
			}
			record := restoreArchiveMetadata(t, source)
			target := restoreArchiveNew(t)
			target.cfg.BackupDir = source.cfg.BackupDir
			target.binding.ManagerID, target.binding.RegistrationID = "example-replacement", "example-replacement"
			executor := &restoreActivationExecutor{}
			require.NoError(t, target.runtime.ConfigureLocal(activity.RuntimeLocalConfig{ApplicationContext: t.Context(), Store: target.binding, Executor: executor}))
			receiver, err := activity.NewRuntimeReceiver(target.runtime)
			require.NoError(t, err)
			transport := &restoreActivationTransport{}
			coordinator, err := activity.NewRuntimeRemoteCoordinator(target.runtime, t.Context(), func(context.Context, activity.RuntimeStoreBinding) (activity.RuntimeRemoteTransport, error) {
				return transport, nil
			}, activity.RuntimeReconcileSchedule{Wait: func(ctx context.Context, _ time.Duration) error { return ctx.Err() }})
			require.NoError(t, err)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
				defer cancel()
				require.NoError(t, coordinator.Close(ctx))
			})
			h, err := activity.NewRuntimeHTTPService(target.runtime, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(),
				Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) {
					t.Fatal("retained restore retry consulted current executor")
					return activity.RuntimeStoreBinding{}, nil
				},
				PrepareTarget: func(context.Context, string, activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
					t.Fatal("restore retry inspected Docker")
					return activity.RuntimeResolvedTarget{}, nil
				}})
			require.NoError(t, err)
			h.SetReady(true)
			mux := http.NewServeMux()
			api := humago.New(mux, huma.DefaultConfig("Restore runtime", "0.0.0"))
			permissions := authz.NewPermissionSet()
			permissions.AddEnv("0", authz.PermProjectsDeploy, authz.PermActivitiesRead)
			api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
				request := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, permissions)
				request = context.WithValue(request, usertypes.CurrentUserContextKey{}, &local.Actor)
				next(huma.WithContext(ctx, request))
			})
			activity.RegisterRuntimeLifecycle(api, target.activities, activity.RuntimeDeployProject, huma.Operation{OperationID: "restore-runtime-retry", Method: "POST", Path: "/environments/{id}/projects/{projectId}/up"})
			request := func() *httptest.ResponseRecorder {
				r := httptest.NewRequest("POST", "/environments/0/projects/"+local.Target.ID+"/up", nil)
				r.Header.Set("Idempotency-Key", local.Key)
				r.Header.Set("Prefer", "respond-async")
				response := httptest.NewRecorder()
				mux.ServeHTTP(response, r)
				return response
			}
			identity := activitytypes.RuntimeRunIdentity{RuntimeExecutorIdentity: activitytypes.RuntimeExecutorIdentity{ManagerID: target.binding.ManagerID, RegistrationID: target.binding.RegistrationID, ExecutorGeneration: target.binding.ExecutorGeneration}, RunID: uuid.NewString(), RequestDigest: strings.Repeat("0", 64)}
			saves, refreshes, unready := 0, 0, 0
			s := NewService(Dependencies{DB: target.db, Config: target.cfg, Activity: target.activities, Store: Store{
				Run: func(context.Context, string) (*backuptypes.SystemBackupRun, error) { return record, nil },
				SaveRun: func(ctx context.Context, _ *backuptypes.SystemBackupRun) error {
					saves++
					persisted, err := session.NewSessionService(target.db).GetSessionByID(ctx, oldSession.ID)
					require.NoError(t, err)
					require.Error(t, session.ValidateActive(persisted))
					require.NotNil(t, persisted.RevokedAt)
					require.Equal(t, 503, request().Code, "receipt repair and revoked sessions do not by themselves reopen admission")
					_, err = receiver.Query(ctx, identity)
					require.ErrorIs(t, err, activity.ErrRuntimeNotReady)
					return ctx.Err()
				}}})
			s.SetExitUnready(func() { unready++ })
			s.SetRestoreSchedulerRefresh(func(ctx context.Context) error {
				refreshes++
				store := target.runtime.LocalStore()
				require.NotEqual(t, source.binding.StoreGeneration, store.StoreGeneration)
				require.NotEqual(t, target.binding.StoreGeneration, store.StoreGeneration)
				require.Equal(t, store.StoreGeneration, store.ExecutorGeneration)
				require.Equal(t, source.binding.ManagerID, store.ManagerID, "replacement uses the restored persisted manager identity")
				metadata, err := receiver.ExecutorIdentity(ctx)
				require.NoError(t, err)
				require.Equal(t, store.ExecutorGeneration, metadata.ExecutorGeneration, "the sole receiver rotated too")
				require.Equal(t, 503, request().Code, "scheduler rebuild precedes intake resume")
				return nil
			})
			work, started := restoreArchiveStart(t, s, target, record)
			ctx, cancel := context.WithTimeout(work, 20*time.Second)
			defer cancel()
			s.executeRestore(ctx, usertypes.Actor{ID: "example-restorer", Username: "Restore operator"}, "", record.ID, started)
			require.Equal(t, 1, saves)
			if fault == "none" {
				require.Equal(t, 1, refreshes)
				require.Zero(t, unready)
				require.NoError(t, CheckPendingRestore(target.cfg))
				response := request()
				require.Equal(t, 200, response.Code, response.Body.String())
				require.Contains(t, response.Body.String(), localID)
				require.Contains(t, response.Body.String(), "runtime.interrupted")
				var restored activity.RuntimeReceipt
				require.NoError(t, target.db.First(&restored, "run_id = ?", localID).Error)
				require.NotNil(t, restored.SettledAt)
				require.Equal(t, new("runtime.interrupted"), restored.ExecutorResultCode)
				require.Equal(t, target.runtime.LocalStore().StoreGeneration, restored.StoreGeneration)
				require.NoError(t, coordinator.Close(ctx))
				restored = activity.RuntimeReceipt{}
				require.NoError(t, target.db.First(&restored, "run_id = ?", remoteID).Error)
				require.Nil(t, restored.SettledAt)
				require.Equal(t, new("unknown"), restored.ExecutorState)
			} else {
				require.Equal(t, 1, unready)
				require.Zero(t, refreshes)
				require.True(t, s.maintenance.Status().Active)
				require.ErrorIs(t, CheckPendingRestore(target.cfg), ErrRestoreRecoveryRequired)
				require.Equal(t, 503, request().Code)
				_, err := receiver.Query(ctx, identity)
				require.ErrorIs(t, err, activity.ErrRuntimeNotReady)
			}
			require.NoError(t, coordinator.Close(ctx))
			require.Zero(t, executor.calls.Load(), "restored local work is never replayed")
			require.Zero(t, transport.calls.Load(), "restored remote work is queried, never executed")
			var fences int64
			require.NoError(t, target.db.Model(&activity.RuntimeReceipt{}).Where("kind = 'not_received'").Count(&fences).Error)
			require.Zero(t, fences, "unready receiver cannot create negative authority")
		})
	}
}
