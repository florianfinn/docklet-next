// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original lost-terminal-response, two-store reopen and real query/retry proof.

package activity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	dto "github.com/florianfinn/docklet-next/types/activity"
	containerDTO "github.com/florianfinn/docklet-next/types/container"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

func TestRecreateLostTerminalHistoryClearBothStoresReopenRealQueryAndRetry(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			f := newRemoteIntegration(t, isEdge)
			daemon := newRecreateHTTP(t)
			f.agent.http.onCall = daemon.serve
			f.agent.containers.ConfigureOperations(f.agent.db.DB, f.agent.store.RegistrationID, false, nil, nil)
			intake, err := activity.NewRuntimeHTTPService(f.hub, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return f.store, nil }, PrepareTarget: func(ctx context.Context, _ string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
				return f.agent.adapter.ResolveRuntimeTarget(ctx, target)
			}, GuardContainer: func(context.Context, string, string) error { return nil }})
			require.NoError(t, err)
			intake.SetReady(true)
			command := f.command(activity.RuntimeRecreateContainer, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
			command.Options.Recreate = &containerDTO.Edit{Name: new("renamed")}
			f.dropTerminal.Store(true)
			admitted, err := f.hub.Admit(t.Context(), command)
			require.NoError(t, err)
			ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
			_, err = f.control.Start(t.Context(), ref, command.Options)
			require.NoError(t, err)
			require.NoError(t, f.control.WaitDispatch(t.Context(), ref.RunID))
			settled, err := f.agent.waitLocal(ref.RunID)
			require.NoError(t, err)
			require.Equal(t, dto.StatusSuccess, settled.Status)
			require.NotNil(t, settled.Result)
			var receipt activity.RuntimeReceipt
			require.NoError(t, f.db.First(&receipt, "run_id=?", ref.RunID).Error)
			identity := dto.RuntimeRunIdentity{RuntimeExecutorIdentity: dto.RuntimeExecutorIdentity{ManagerID: f.store.ManagerID, RegistrationID: f.store.RegistrationID, ExecutorGeneration: f.store.ExecutorGeneration}, RunID: ref.RunID, RequestDigest: receipt.RequestDigest}
			_, err = f.sender.Query(t.Context(), identity)
			require.Error(t, err)
			require.Positive(t, f.lostTerminal.Load())
			require.NoError(t, f.control.Close(t.Context()))
			require.NoError(t, f.db.First(&receipt, "run_id=?", ref.RunID).Error)
			require.Nil(t, receipt.ExecutorSettledAt, "the lost terminal answer cannot settle the hub")
			for _, db := range []*database.DB{f.db, f.agent.db} {
				require.NoError(t, db.Exec("DELETE FROM activity_messages").Error)
				require.NoError(t, db.Exec("DELETE FROM activities").Error)
				path := db.Path()
				require.NoError(t, db.Close())
				reopened, err := database.InitializeHub(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
				require.NoError(t, err)
				*db = *reopened
			}
			f.dropTerminal.Store(false)
			outcome, err := f.sender.Query(t.Context(), identity)
			require.NoError(t, err)
			require.Equal(t, settled.Result, outcome.Detail.Activity.Result)
			service := activity.NewActivityService(f.db, nil)
			runtime := activity.NewRuntimeService(service, activity.RuntimeResolverFunc(func(context.Context, activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
				t.Fatal("retained retry inspected removed old ID")
				return activity.RuntimeResolvedTarget{}, nil
			}))
			_, err = runtime.InitializeGeneration(t.Context())
			require.NoError(t, err)
			control, err := activity.NewRuntimeRemoteCoordinator(runtime, t.Context(), func(context.Context, activity.RuntimeStoreBinding) (activity.RuntimeRemoteTransport, error) {
				return f.sender, nil
			}, activity.RuntimeReconcileSchedule{Wait: func(ctx context.Context, _ time.Duration) error { return ctx.Err() }})
			require.NoError(t, err)
			defer control.Close(t.Context())
			snapshot, err := control.Refresh(t.Context(), ref)
			require.NoError(t, err)
			require.Equal(t, settled.Result, snapshot.Result)
			restoredHTTP, err := activity.NewRuntimeHTTPService(runtime, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) {
				t.Fatal("retained retry fetched executor metadata")
				return f.store, nil
			}, PrepareTarget: func(context.Context, string, activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
				t.Fatal("retained retry inspected target")
				return activity.RuntimeResolvedTarget{}, nil
			}})
			require.NoError(t, err)
			restoredHTTP.SetReady(true)
			mux := http.NewServeMux()
			api := humago.New(mux, huma.DefaultConfig("Recreate retry", "0.0.0"))
			api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
				request := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
				request = context.WithValue(request, userDTO.CurrentUserContextKey{}, &command.Actor)
				next(huma.WithContext(ctx, request))
			})
			activity.RegisterRuntimeLifecycle(api, service, activity.RuntimeRecreateContainer, huma.Operation{OperationID: "retry-recreate", Method: http.MethodPost, Path: "/environments/{id}/containers/{containerId}/recreate"})
			before := daemon.counts()
			request := httptest.NewRequest(http.MethodPost, "/environments/"+f.store.EnvironmentID+"/containers/"+runtimeContainerID+"/recreate", bytes.NewBufferString(`{"edit":{"name":"renamed"}}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", command.Key)
			request.Header.Set("Prefer", "respond-async")
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			require.Equal(t, 200, response.Code, response.Body.String())
			var retained struct{ Data dto.RuntimeAdmission }
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &retained))
			require.Equal(t, ref.RunID, retained.Data.ActivityID)
			require.Equal(t, &containerDTO.RecreateResult{OldID: runtimeContainerID, NewID: recreateNewID, OldName: "app", NewName: "renamed"}, retained.Data.Activity.Result)
			require.Equal(t, before, daemon.counts())
			require.Equal(t, 1, before["create"])
			require.EqualValues(t, 1, f.executeCalls.Load())
		})
	}
}
