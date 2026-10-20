// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Legacy progress/list projections retain receipt knowledge and hub authority.

package activity

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestRuntimeActivationProgressAndListKeepLocalKnowledge(t *testing.T) {
	probe := slotsNewProbe(t)
	f := slotsFixture(t, t.Context(), new(1), func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error { return probe.run(ctx) })
	slotsCleanup(t, f, probe)
	require.NoError(t, f.service.BindActivityOwner())
	events, _, unsubscribe := f.store.svc.Subscribe("0")
	defer unsubscribe()
	c := f.command()
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	b1Await(t, probe.entered)
	owner := b2Owner(t, f, admitted.ActivityID)
	updated, err := f.store.svc.UpdateActivity(owner.ctx, admitted.ActivityID, UpdateActivityRequest{Step: new("fixture-progress")})
	require.NoError(t, err)
	require.NotNil(t, updated.RuntimeKnowledge)
	require.Equal(t, activitytypes.RuntimeOutcomeKnown, updated.OutcomeState)
	_, err = f.store.svc.AppendMessages(owner.ctx, admitted.ActivityID, []AppendActivityMessageRequest{{Message: "fixture output", Code: string(activitytypes.CodeActivityMessage), Step: "fixture-output"}})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		select {
		case event := <-events:
			if event.Activity != nil && event.Activity.ID == admitted.ActivityID {
				require.NotNil(t, event.Activity.RuntimeKnowledge)
				require.Equal(t, activitytypes.RuntimeOutcomeKnown, event.Activity.OutcomeState)
				if event.Activity.Step == "fixture-output" {
					goto projected
				}
			}
		case <-ctx.Done():
			t.Fatal("owned progress snapshot did not arrive")
		}
	}
projected:
	rows, _, err := f.store.svc.ListActivitiesPaginated(t.Context(), "0", pagination.QueryParams{Limit: 50})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].RuntimeKnowledge)
	require.Equal(t, activitytypes.RuntimeOutcomeKnown, rows[0].OutcomeState)
	probe.unblock()
	_, err = b2Wait(t, f, c, admitted.ActivityID)
	require.NoError(t, err)
}

func TestRuntimeActivationRemoteListDeduplicatesWithoutSettling(t *testing.T) {
	probe := &remoteTransportProbe{}
	f := remoteFixture(t, probe, RuntimeReconcileSchedule{})
	require.NoError(t, f.service.BindActivityOwner())
	f.start(t)
	snapshot, err := f.control.Snapshot(t.Context(), f.ref)
	require.NoError(t, err)
	require.Equal(t, activitytypes.RuntimeOutcomeUnknown, snapshot.OutcomeState)
	observed := snapshot.Activity
	observed.Status = activitytypes.StatusSuccess
	observed.RuntimeKnowledge = &activitytypes.RuntimeKnowledge{OutcomeState: activitytypes.RuntimeOutcomeKnown}
	rows, page, err := f.store.svc.ListRemoteActivities(t.Context(), f.ref.Store.EnvironmentID, []activitytypes.Activity{observed}, 1, pagination.QueryParams{Limit: 50})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 1, page.TotalItems)
	require.Equal(t, f.ref.RunID, rows[0].ID)
	require.Equal(t, activitytypes.StatusRunning, rows[0].Status)
	require.NotNil(t, rows[0].RuntimeKnowledge)
	require.Equal(t, activitytypes.RuntimeOutcomeUnknown, rows[0].OutcomeState)
	require.Nil(t, f.receipt(t).SettledAt)
	require.NoError(t, f.control.Close(t.Context()))
	require.EqualValues(t, 1, probe.executes.Load())
}
