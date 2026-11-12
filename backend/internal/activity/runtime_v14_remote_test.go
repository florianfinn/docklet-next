// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original real Direct/Edge v14 allowlist coverage; S3 executes real update and restore workflows.

package activity_test

import (
	"context"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	dto "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestV14ContainerOperationsDirectAndEdge(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		for _, manual := range []bool{false, true} {
			t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge]+map[bool]string{false: "/detected", true: "/manual"}[manual], func(t *testing.T) {
				daemon, reg := newContainerImageHTTP(t)
				if manual {
					old := daemon.base.containers[runtimeContainerID]
					old.Labels = map[string]string{"io.kubernetes.pod.name": "app"}
					daemon.base.containers[runtimeContainerID] = old
				}
				f := newRemoteIntegration(t, isEdge, func(agent *runtimeServicesFixture) { configureContainerImageFixture(t, agent, daemon, reg) })
				guard := &origin.Store{DB: f.db.DB, LocalRegistration: "hub"}
				if manual {
					require.NoError(t, f.db.Create(&origin.Override{RegistrationID: f.store.RegistrationID, ContainerName: "app", DockerID: runtimeContainerID, Origin: "standalone", AssignedBy: "operator"}).Error)
				}
				intake, e := activity.NewRuntimeHTTPService(f.hub, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return f.store, nil }, PrepareTarget: func(ctx context.Context, _ string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
					return f.agent.adapter.ResolveRuntimeTarget(ctx, target)
				}, GuardContainer: func(ctx context.Context, _ string, id string) error {
					current, e := f.agent.containers.GetContainerByID(ctx, id)
					if e != nil {
						return e
					}
					return guard.Guard(ctx, f.store.RegistrationID, origin.Identity{ID: current.ID, Name: "app", Labels: current.Config.Labels})
				}})
				require.NoError(t, e)
				intake.SetReady(true)
				currentID := runtimeContainerID
				for i, operation := range []activity.RuntimeOperation{activity.RuntimeUpdateContainer, activity.RuntimeRestoreContainerImage} {
					command := f.command(operation, activity.RuntimeTarget{Kind: "container", ID: currentID})
					accepted, e := f.hub.Admit(t.Context(), command)
					require.NoError(t, e)
					ref := activity.RuntimeRunReference{Store: f.store, RunID: accepted.ActivityID}
					_, e = f.control.Start(t.Context(), ref, command.Options)
					require.NoError(t, e)
					ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
					require.NoError(t, f.control.WaitDispatch(ctx, ref.RunID))
					_, e = f.agent.waitLocal(ref.RunID)
					require.NoError(t, e)
					snapshot, e := f.control.Wait(ctx, ref, nil)
					cancel()
					require.NoError(t, e)
					require.Equal(t, dto.StatusSuccess, snapshot.Status, "%+v", snapshot.ImageLifecycle)
					require.Equal(t, dto.RuntimeOutcomeKnown, snapshot.OutcomeState)
					require.NotNil(t, snapshot.ImageLifecycle)
					require.True(t, snapshot.ImageLifecycle.Verified)
					require.Empty(t, snapshot.ImageLifecycle.Code)
					require.NotNil(t, snapshot.Result)
					require.Equal(t, currentID, snapshot.Result.OldID)
					require.Equal(t, "app", snapshot.Result.OldName)
					require.Equal(t, "app", snapshot.Result.NewName)
					currentID = snapshot.Result.NewID
					details, e := f.agent.containers.GetContainerDetails(t.Context(), currentID)
					require.NoError(t, e)
					if i == 0 {
						require.NotNil(t, details.PreviousImage)
						require.Equal(t, daemon.oldImage, details.PreviousImage.ImageID)
						require.Equal(t, daemon.ref, details.PreviousImage.ImageRef)
					} else {
						require.Nil(t, details.PreviousImage)
						require.Equal(t, daemon.oldImage, details.ImageID)
					}
					require.EqualValues(t, i+1, f.executeCalls.Load(), "each operation crosses runtime.execute")
					daemon.mu.Lock()
					pulls, tags, creates := daemon.pulls, daemon.tags, daemon.creates
					daemon.mu.Unlock()
					require.Equal(t, 1, pulls, "update pulls once; restore adds no pull")
					require.Equal(t, i, tags, "only restore retags the previous image")
					require.Equal(t, i+1, creates, "each operation recreates exactly once")
				}
				if manual {
					var moved origin.Override
					require.NoError(t, f.db.First(&moved, "registration_id=?", f.store.RegistrationID).Error)
					require.Equal(t, currentID, moved.DockerID)
				}
				// Keep the pre-existing zero-secret wire expectations in the sender fixture;
				// only the temporary unavailable result has been replaced by real execution.
			})
		}
	}
}
