// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Actual executeRestore cancellation and seal-failure continuity regressions.

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
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRuntimeActivationActualRestoreAbort(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		for _, fault := range []string{"cancelled", "seal", "resume-read"} {
			t.Run(transport+"/"+fault, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
				defer cancel()
				source := restoreArchiveNew(t)
				record := restoreArchiveMetadata(t, source)
				target := restoreArchiveNew(t)
				target.cfg.BackupDir = source.cfg.BackupDir
				var saves, unready atomic.Int32
				s := NewService(Dependencies{DB: target.db, Config: target.cfg, Activity: target.activities, Store: Store{
					Run:     func(context.Context, string) (*backuptypes.SystemBackupRun, error) { return record, nil },
					SaveRun: func(context.Context, *backuptypes.SystemBackupRun) error { saves.Add(1); return nil },
				}})
				s.SetExitUnready(func() { unready.Add(1) })
				work, started := restoreArchiveStart(t, s, target, record)
				// An uncancelled caller context isolates the real SealCancellation failure
				// from the preceding workCtx.Err branch after the registration is removed.
				if fault == "seal" {
					work = context.WithoutCancel(work)
				}
				remote := restoreAbortTransport(t, ctx, target, transport)
				entered := make(chan struct{})
				var localCalls atomic.Int32
				var oldCommand activity.RuntimeCommand
				var oldID string
				require.NoError(t, target.runtime.ConfigureLocal(activity.RuntimeLocalConfig{ApplicationContext: ctx, Store: target.binding, Executor: restoreExecutorFunc(func(ownerCtx context.Context, _ activity.RuntimeCommand, _ io.Writer) error {
					if localCalls.Add(1) != 1 {
						return nil
					}
					close(entered)
					<-ownerCtx.Done()
					remote.release()
					agentRef := activity.RuntimeRunReference{Store: remote.agent.binding, RunID: oldID}
					_, err := remote.agent.runtime.WaitLocal(ctx, agentRef)
					if err != nil {
						return err
					}
					if fault == "seal" {
						_, err = target.activities.CompleteActivity(context.WithoutCancel(work), started.ID, activitytypes.StatusCancelled, "Synthetic completed restore owner", nil)
						return err
					}
					if fault == "resume-read" {
						if err := target.db.Exec("CREATE TRIGGER fixture_resume_fault BEFORE UPDATE OF executor_state ON runtime_receipts WHEN NEW.executor_state='unknown' BEGIN SELECT RAISE(ABORT,'synthetic resume refusal'); END").Error; err != nil {
							return err
						}
					}
					target.activities.RequestCancel(started.ID)
					return nil
				})}))
				receiver, err := activity.NewRuntimeReceiver(target.runtime)
				require.NoError(t, err)
				coordinator, err := activity.NewRuntimeRemoteCoordinator(target.runtime, ctx, func(context.Context, activity.RuntimeStoreBinding) (activity.RuntimeRemoteTransport, error) {
					return remote.sender, nil
				}, activity.RuntimeReconcileSchedule{Wait: func(ctx context.Context, _ time.Duration) error { return ctx.Err() }})
				require.NoError(t, err)
				defer func() {
					join, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
					defer stop()
					require.NoError(t, coordinator.Close(join))
				}()
				h, err := activity.NewRuntimeHTTPService(target.runtime, activity.RuntimeHTTPPorts{ApplicationContext: ctx,
					Store: func(_ context.Context, id string) (activity.RuntimeStoreBinding, error) {
						if id == "0" {
							return target.runtime.LocalStore(), nil
						}
						return remote.store, nil
					},
					PrepareTarget: func(_ context.Context, _ string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
						return activity.RuntimeResolvedTarget{Target: target}, nil
					},
				})
				require.NoError(t, err)
				h.SetReady(true)
				mux := http.NewServeMux()
				api := humago.New(mux, huma.DefaultConfig("Restore abort runtime", "0.0.0"))
				api.UseMiddleware(func(c huma.Context, next func(huma.Context)) {
					ps := authz.NewPermissionSet()
					ps.AddEnv(remote.store.EnvironmentID, authz.PermContainersStart, authz.PermActivitiesRead)
					request := context.WithValue(c.Context(), middleware.ContextKeyUserPermissions, ps)
					actor := usertypes.Actor{ID: "example-owner", Username: "Operator"}
					request = context.WithValue(request, usertypes.CurrentUserContextKey{}, &actor)
					next(huma.WithContext(c, request))
				})
				activity.RegisterRuntimeLifecycle(api, target.activities, activity.RuntimeStartContainer, huma.Operation{OperationID: "restore-abort-start", Method: http.MethodPost, Path: "/environments/{id}/containers/{containerId}/start"})
				request := func() *httptest.ResponseRecorder {
					r := httptest.NewRequest(http.MethodPost, "/environments/"+remote.store.EnvironmentID+"/containers/"+strings.Repeat("b", 64)+"/start", nil).WithContext(ctx)
					r.Header.Set("Prefer", "respond-async")
					r.Header.Set("Idempotency-Key", uuid.NewString())
					w := httptest.NewRecorder()
					mux.ServeHTTP(w, r)
					return w
				}
				oldCommand = target.command("old-remote")
				oldCommand.Store = remote.store
				oldCommand.Operation = activity.RuntimeStartContainer
				oldCommand.Target = activity.RuntimeTarget{Kind: "container", ID: strings.Repeat("a", 64)}
				oldID = target.admit(t, oldCommand)
				_, err = coordinator.Start(ctx, activity.RuntimeRunReference{Store: oldCommand.Store, RunID: oldID}, oldCommand.Options)
				require.NoError(t, err)
				select {
				case status := <-remote.accepted:
					require.Equal(t, http.StatusOK, status, "actual receiver must accept before acknowledgment loss")
				case <-ctx.Done():
					t.Fatal("old Execute did not reach actual transport")
				}
				local := target.command("local-join")
				local.Operation = activity.RuntimeStartContainer
				local.Target = activity.RuntimeTarget{Kind: "container", ID: strings.Repeat("c", 64)}
				localID := target.admit(t, local)
				restoreAbortWait(t, ctx, entered, "local runtime owner did not start")
				beforeBinding := target.runtime.LocalStore()
				s.executeRestore(work, usertypes.Actor{ID: "example-restorer", Username: "Restore operator"}, "", record.ID, started)
				require.Zero(t, saves.Load(), "aborted restore must not install or save the archive")
				require.Equal(t, beforeBinding, target.runtime.LocalStore(), "pre-replacement abort preserves receiver generation")
				require.NoError(t, CheckPendingRestore(target.cfg))
				require.NoError(t, coordinator.WaitDispatch(ctx, oldID))
				_, err = target.runtime.WaitLocal(ctx, activity.RuntimeRunReference{Store: local.Store, RunID: localID})
				require.NoError(t, err)
				if fault == "resume-read" {
					var restore activity.Activity
					require.NoError(t, target.db.First(&restore, "id = ?", started.ID).Error)
					assert.Equal(t, activitytypes.StatusRunning, restore.Status, "failed recovery leaves restore unresolved")
					assert.Nil(t, restore.EndedAt)
					var finals int64
					require.NoError(t, target.db.Model(&activity.ActivityMessage{}).Where("activity_id = ? AND code IN ?", started.ID, []string{"activity.success", "activity.failed", "activity.cancelled"}).Count(&finals).Error)
					require.Zero(t, finals, "failed recovery must not publish a final restore message")
					require.EqualValues(t, 1, unready.Load())
					require.True(t, s.maintenance.Status().Active)
					require.Equal(t, "blocked", s.maintenance.Status().Phase)
					require.Equal(t, 503, request().Code)
					// Metadata reads are allowed during successful scheduler rebuilding, but
					// negative Query authority must remain closed on failed recovery.
					identity := activitytypes.RuntimeRunIdentity{RuntimeExecutorIdentity: activitytypes.RuntimeExecutorIdentity{ManagerID: beforeBinding.ManagerID, RegistrationID: beforeBinding.RegistrationID, ExecutorGeneration: beforeBinding.ExecutorGeneration}, RunID: uuid.NewString(), RequestDigest: strings.Repeat("0", 64)}
					_, err = receiver.Query(ctx, identity)
					require.ErrorIs(t, err, activity.ErrRuntimeNotReady)
					require.NoError(t, target.db.Exec("DROP TRIGGER fixture_resume_fault").Error)
					require.EqualValues(t, 1, remote.executes.Load())
					return
				}
				require.Zero(t, unready.Load())
				require.False(t, s.maintenance.Status().Active)
				old := restoreAbortSnapshot(t, ctx, target, coordinator, oldCommand, oldID)
				require.Equal(t, activitytypes.StatusSuccess, old.Status, "resume queries the interrupted dispatch without replay")
				require.Equal(t, activitytypes.RuntimeOutcomeKnown, old.OutcomeState)
				require.Positive(t, remote.queries.Load())
				response := request()
				require.Equal(t, 202, response.Code, response.Body.String())
				var rows []activity.RuntimeReceipt
				require.NoError(t, target.db.WithContext(ctx).Where("environment_id = ? AND run_id <> ? AND kind = 'receipt'", remote.store.EnvironmentID, oldID).Find(&rows).Error)
				require.Len(t, rows, 1)
				fresh := rows[0]
				require.NoError(t, coordinator.WaitDispatch(ctx, fresh.RunID))
				_, err = remote.agent.runtime.WaitLocal(ctx, activity.RuntimeRunReference{Store: remote.agent.binding, RunID: fresh.RunID})
				require.NoError(t, err)
				freshCommand := oldCommand
				freshCommand.Target.ID = *fresh.TargetID
				result := restoreAbortSnapshot(t, ctx, target, coordinator, freshCommand, fresh.RunID)
				require.Equal(t, activitytypes.StatusSuccess, result.Status)
				require.Equal(t, activitytypes.RuntimeOutcomeKnown, result.OutcomeState)
				require.EqualValues(t, 2, remote.executes.Load(), "one old and one new Execute after joined dispatch barriers")
				require.EqualValues(t, 2, remote.effects.Load())
				require.NoError(t, target.activities.ResumeRuntimeIntake(ctx))
				require.NoError(t, coordinator.WaitDispatch(ctx, fresh.RunID))
				require.EqualValues(t, 2, remote.executes.Load(), "repeated resume cannot replay")
				removed := false
				require.NoError(t, target.runtime.RemoveEnvironment(ctx, "example-empty", func(*gorm.DB) error { removed = true; return nil }))
				require.True(t, removed, "environment removal is no longer storage-paused")
				expired := time.Now().UTC().Add(-25 * time.Hour)
				tomb := activity.RuntimeReceipt{RunID: uuid.NewString(), Kind: "not_received", StoreGeneration: beforeBinding.StoreGeneration, ExecutorGeneration: beforeBinding.ExecutorGeneration, ManagerID: beforeBinding.ManagerID, RegistrationID: beforeBinding.RegistrationID, EnvironmentID: "0", RequestDigest: "synthetic-expired", SettledAt: &expired, FinalWriteAt: expired}
				require.NoError(t, target.db.Create(&tomb).Error)
				require.NoError(t, target.runtime.PruneRuntime(ctx))
				var remaining int64
				require.NoError(t, target.db.Model(&activity.RuntimeReceipt{}).Where("run_id = ?", tomb.RunID).Count(&remaining).Error)
				require.Zero(t, remaining, "compact pruning resumed")
				// The resumed maintenance lifetime must retry only frozen terminal writes.
				require.NoError(t, target.db.Exec("CREATE TRIGGER fixture_post_resume_settlement BEFORE UPDATE OF settled_at ON runtime_receipts WHEN NEW.settled_at IS NOT NULL BEGIN SELECT RAISE(ABORT,'synthetic terminal refusal'); END").Error)
				nextLocal := local
				nextLocal.Key = uuid.NewString()
				nextLocal.Target.ID = strings.Repeat("d", 64)
				nextID := target.admit(t, nextLocal)
				nextRef := activity.RuntimeRunReference{Store: nextLocal.Store, RunID: nextID}
				_, err = target.runtime.WaitLocal(ctx, nextRef)
				require.Error(t, err)
				require.NoError(t, target.db.Exec("DROP TRIGGER fixture_post_resume_settlement").Error)
				events, _, unsubscribe := target.activities.Subscribe("0")
				defer unsubscribe()
				life, stopMaintenance := context.WithCancel(ctx)
				maintenanceDone := make(chan error, 1)
				go func() { maintenanceDone <- target.runtime.RunMaintenance(life) }()
				defer func() {
					stopMaintenance()
					select {
					case err := <-maintenanceDone:
						require.NoError(t, err)
					case <-time.After(5 * time.Second):
						t.Error("maintenance did not join")
					}
				}()
				for {
					detail, err := target.runtime.GetRuntimeDetail(ctx, nextRef)
					require.NoError(t, err)
					if detail.Activity.Status == activitytypes.StatusSuccess {
						break
					}
					select {
					case <-events:
					case <-ctx.Done():
						t.Fatal("post-resume frozen result was not retried")
					}
				}
				require.EqualValues(t, 2, localCalls.Load(), "settlement retry cannot execute the local workflow again")
			})
		}
	}
}

func TestRuntimeActivationRestoreWaitCancellationPreservesReadiness(t *testing.T) {
	for _, state := range []struct{ ready, busy bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		ready := state.ready
		t.Run(map[bool]string{false: "already-unready", true: "ready"}[ready]+map[bool]string{false: "-idle", true: "-draining"}[state.busy], func(t *testing.T) {
			applicationCtx, stopApplication := context.WithCancel(t.Context())
			defer stopApplication()
			source := restoreArchiveNew(t)
			record := restoreArchiveMetadata(t, source)
			target := restoreArchiveNew(t)
			target.cfg.BackupDir = source.cfg.BackupDir
			executor := &restoreActivationExecutor{}
			require.NoError(t, target.runtime.ConfigureLocal(activity.RuntimeLocalConfig{ApplicationContext: applicationCtx, Store: target.binding, Executor: executor}))
			receiver, err := activity.NewRuntimeReceiver(target.runtime)
			require.NoError(t, err)
			h, err := activity.NewRuntimeHTTPService(target.runtime, activity.RuntimeHTTPPorts{ApplicationContext: applicationCtx, Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return target.binding, nil }, PrepareTarget: func(_ context.Context, _ string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
				return activity.RuntimeResolvedTarget{Target: target}, nil
			}})
			require.NoError(t, err)
			h.SetReady(ready)
			s := NewService(Dependencies{DB: target.db, Config: target.cfg, Activity: target.activities, Store: Store{Run: func(context.Context, string) (*backuptypes.SystemBackupRun, error) { return record, nil }}})
			work, started := restoreArchiveStart(t, s, target, record)
			if state.busy {
				leave, err := s.maintenance.Admit()
				require.NoError(t, err)
				defer leave()
			}
			s.maintenance.OnBegin(func() { target.activities.RequestCancel(started.ID) })
			// Fixture setup must not consume the cancellation join budget.
			ctx, cancel := context.WithTimeout(applicationCtx, 5*time.Second)
			defer cancel()
			done := make(chan struct{})
			go func() {
				defer close(done)
				s.executeRestore(work, usertypes.Actor{ID: "example-restorer", Username: "Restore operator"}, "", record.ID, started)
			}()
			restoreAbortWait(t, ctx, done, "cancelled maintenance wait did not join")
			require.False(t, s.maintenance.Status().Active)
			identity := activitytypes.RuntimeRunIdentity{RuntimeExecutorIdentity: activitytypes.RuntimeExecutorIdentity{ManagerID: target.binding.ManagerID, RegistrationID: target.binding.RegistrationID, ExecutorGeneration: target.binding.ExecutorGeneration}, RunID: uuid.NewString(), RequestDigest: strings.Repeat("0", 64)}
			_, err = receiver.Query(ctx, identity)
			if ready {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, activity.ErrRuntimeNotReady)
			}
			require.Equal(t, target.binding, target.runtime.LocalStore())
			require.Zero(t, executor.calls.Load())
			require.NoError(t, CheckPendingRestore(target.cfg))
		})
	}
}
