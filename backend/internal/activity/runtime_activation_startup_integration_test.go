// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Real Fx startup receipt repair and pre-readiness refusal.

package activity_test

import (
	"context"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRuntimeActivationFxStartupRepairBeforeReceiverAuthority(t *testing.T) {
	for _, fault := range []bool{false, true} {
		t.Run(map[bool]string{false: "ready", true: "repair-refused"}[fault], func(t *testing.T) {
			var run, key string
			var command activity.RuntimeCommand
			var identity activitytypes.RuntimeRunIdentity
			options := activationSetup{BeforeStart: func(f *runtimeServicesFixture, _ *activity.RuntimeHTTPService, receiver *activity.RuntimeReceiver) {
				// One dormant service for a separate historical ActivityService writes a
				// persisted pre-process row. The production Fx service owns every worker.
				previous := activity.NewRuntimeService(activity.NewActivityService(f.db, f.settings), activity.RuntimeResolverFunc(func(_ context.Context, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
					return activity.RuntimeResolvedTarget{Target: target}, nil
				}))
				command = activity.RuntimeCommand{Store: f.store, Key: uuid.NewString(), Actor: usertypes.Actor{ID: "historical-owner", Username: "Historical operator"}, Operation: activity.RuntimeStartContainer, Target: activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID}}
				admitted, err := previous.Admit(t.Context(), command)
				require.NoError(t, err)
				run, key = admitted.ActivityID, command.Key
				identity = activitytypes.RuntimeRunIdentity{RuntimeExecutorIdentity: activitytypes.RuntimeExecutorIdentity{ManagerID: f.store.ManagerID, RegistrationID: f.store.RegistrationID, ExecutorGeneration: f.store.ExecutorGeneration}, RunID: uuid.NewString(), RequestDigest: strings.Repeat("0", 64)}
				_, err = receiver.Query(t.Context(), identity)
				require.ErrorIs(t, err, activity.ErrRuntimeNotReady)
				var count int64
				require.NoError(t, f.db.Model(&activity.RuntimeReceipt{}).Where("kind = 'not_received'").Count(&count).Error)
				require.Zero(t, count)
				if fault {
					require.NoError(t, f.db.Exec("CREATE TRIGGER fixture_activation_startup BEFORE UPDATE OF executor_state ON runtime_receipts WHEN NEW.executor_state='failed' BEGIN SELECT RAISE(ABORT,'synthetic startup repair fault'); END").Error)
				}
			}}
			if fault {
				options.ExpectedStartError = "synthetic startup repair fault"
			}
			f := activationApp(t, false, "", options)
			var receipt activity.RuntimeReceipt
			require.NoError(t, f.services.db.First(&receipt, "run_id = ?", run).Error)
			if fault {
				require.Nil(t, receipt.SettledAt)
				_, err := f.receiver.Query(t.Context(), identity)
				require.ErrorIs(t, err, activity.ErrRuntimeNotReady)
				status, body := activationCall(t, f, "/api/environments/0/containers/"+runtimeContainerID+"/start", key, "respond-async", nil)
				require.Equal(t, 503, status, string(body))
				require.NotContains(t, string(body), run)
				require.NoError(t, f.services.db.Exec("DROP TRIGGER fixture_activation_startup").Error)
			} else {
				require.NotNil(t, receipt.SettledAt)
				require.Equal(t, new(string(activitytypes.CodeRuntimeInterrupted)), receipt.ExecutorResultCode)
				result, err := f.receiver.Query(t.Context(), identity)
				require.NoError(t, err)
				require.Equal(t, activitytypes.RuntimeAnswerNotReceived, result.State)
				ref, found, err := f.services.runtime.Reference(t.Context(), "0", run)
				require.NoError(t, err)
				require.True(t, found)
				detail, err := f.services.runtime.GetRuntimeDetail(t.Context(), ref)
				require.NoError(t, err)
				require.Equal(t, activitytypes.StatusFailed, detail.Activity.Status)
				require.Equal(t, activitytypes.RuntimeOutcomeKnown, detail.Activity.OutcomeState)
			}
			require.Empty(t, runtimeEffectEvents(f.services.http.snapshot()), "startup never re-executes an abandoned receipt")
		})
	}
}

func TestRuntimeActivationReceiverHasOneConstructionOwner(t *testing.T) {
	f := activationApp(t, true, uuid.NewString())
	duplicate, err := activity.NewRuntimeReceiver(f.services.runtime)
	require.Error(t, err)
	require.Nil(t, duplicate)
}
