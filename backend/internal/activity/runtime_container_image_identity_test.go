// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original frozen-name regression across actual Direct/Edge runtime receivers.

package activity_test

import (
	"context"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	containers "github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/stretchr/testify/require"
)

func TestContainerImageRenameAfterAdmissionDirectEdge(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		for _, op := range []activity.RuntimeOperation{activity.RuntimeUpdateContainer, activity.RuntimeRestoreContainerImage} {
			t.Run(map[bool]string{false: "direct/", true: "edge/"}[isEdge]+string(op), func(t *testing.T) {
				daemon, reg := newContainerImageHTTP(t)
				f := newRemoteIntegration(t, isEdge, func(agent *runtimeServicesFixture) { configureContainerImageFixture(t, agent, daemon, reg) })
				h, err := activity.NewRuntimeHTTPService(f.hub, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return f.store, nil }, PrepareTarget: func(ctx context.Context, _ string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
					return f.agent.adapter.ResolveRuntimeTarget(ctx, target)
				}, GuardContainer: func(context.Context, string, string) error { return nil }})
				require.NoError(t, err)
				h.SetReady(true)
				// Both names have an otherwise usable slot: a missing slot must not mask
				// a receiver that wrongly moves the intent to the container's fresh name.
				if op == activity.RuntimeRestoreContainerImage {
					for _, name := range []string{"app", "renamed"} {
						require.NoError(t, f.agent.db.Create(&containers.PreviousImage{ContainerName: name, ImageID: daemon.newImage, ImageRef: daemon.ref, RunID: "previous-run", CreatedAt: time.Now().UTC(), OwnerContainerID: runtimeContainerID}).Error)
					}
				}
				command := f.command(op, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
				accepted, err := f.hub.Admit(t.Context(), command)
				require.NoError(t, err)
				daemon.base.mu.Lock()
				old := daemon.base.containers[runtimeContainerID]
				old.Name = "renamed"
				daemon.base.containers[runtimeContainerID] = old
				daemon.base.mu.Unlock()
				ref := activity.RuntimeRunReference{Store: f.store, RunID: accepted.ActivityID}
				_, err = f.control.Start(t.Context(), ref, command.Options)
				require.NoError(t, err)
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				defer cancel()
				require.NoError(t, f.control.WaitDispatch(ctx, ref.RunID))
				_, localErr := f.agent.waitLocal(ref.RunID)
				require.Empty(t, daemon.base.counts(), "the frozen name must be checked before workload effects")
				daemon.mu.Lock()
				pulls, tags, creates := daemon.pulls, daemon.tags, daemon.creates
				daemon.mu.Unlock()
				require.Zero(t, pulls)
				require.Zero(t, tags)
				require.Zero(t, creates)
				require.NoError(t, localErr)
				snapshot, err := f.control.Wait(ctx, ref, nil)
				require.NoError(t, err)
				require.NotNil(t, snapshot.ImageLifecycle)
				require.Equal(t, "update.target-changed", snapshot.ImageLifecycle.Code)
				require.EqualValues(t, 1, f.executeCalls.Load())
			})
		}
	}
}
