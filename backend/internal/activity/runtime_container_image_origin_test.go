// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original per-origin admission refusal and post-admission effect guard proof.

package activity_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	containers "github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	dto "github.com/florianfinn/docklet-next/types/container"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/cgroup"
	"go.getarcane.app/updater/labels"
)

func TestContainerImageOriginAdmission(t *testing.T) {
	for _, op := range []activity.RuntimeOperation{activity.RuntimeUpdateContainer, activity.RuntimeRestoreContainerImage} {
		for _, kind := range []string{"unraid", "uncertain", "compose", "self"} {
			t.Run(string(op)+"/"+kind, func(t *testing.T) {
				f := newRemoteIntegration(t, false)
				id := runtimeContainerID
				fields := map[string]string{}
				switch kind {
				case "unraid":
					fields["net.unraid.docker.managed"] = ""
				case "uncertain":
					fields["io.kubernetes.pod.name"] = "app"
				case "compose":
					fields["com.docker.compose.project"] = "example"
					fields["com.docker.compose.service"] = "app"
				case "self":
					fields["com.getarcaneapp.arcane"] = "true"
					if current, err := cgroup.CurrentContainerID(); err == nil && len(current) == 64 {
						id = current
					}
				}
				daemon := newRecreateHTTP(t)
				delete(daemon.containers, runtimeContainerID)
				daemon.containers[id] = recreateObserved{ID: id, Name: "app", Labels: fields, Running: true}
				f.agent.http.onCall = daemon.serve
				guard := containers.NewContainerService(nil, nil, nil, nil, nil)
				guard.ConfigureOperations(f.db.DB, "hub", true, func(ctx context.Context, _ string, id string) (origin.Identity, error) {
					inspected, e := f.agent.containers.GetContainerByID(ctx, id)
					if e != nil {
						return origin.Identity{}, e
					}
					self, selfErr := cgroup.CurrentContainerID()
					return origin.Identity{ID: inspected.ID, Name: strings.TrimPrefix(inspected.Name, "/"), Labels: inspected.Config.Labels, Protected: labels.ShouldDisableArcaneServerRedeploy(inspected.Config.Labels, inspected.ID, self, selfErr)}, nil
				}, nil)
				h, err := activity.NewRuntimeHTTPService(f.hub, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return f.store, nil }, PrepareTarget: func(ctx context.Context, _ string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
					return f.agent.adapter.ResolveRuntimeTarget(ctx, target)
				}, GuardContainer: guard.GuardRuntimeOrigin})
				require.NoError(t, err)
				h.SetReady(true)
				_, err = f.hub.Admit(t.Context(), f.command(op, activity.RuntimeTarget{Kind: "container", ID: id}))
				var problem *dto.Problem
				require.ErrorAs(t, err, &problem)
				require.Equal(t, "container.origin-locked", problem.Code)
				require.Equal(t, 409, problem.Status)
				require.Empty(t, daemon.counts())
				require.Zero(t, f.executeCalls.Load())
				var receipts int64
				require.NoError(t, f.db.Table("runtime_receipts").Count(&receipts).Error)
				require.Zero(t, receipts)
			})
		}
	}
}

func TestContainerImageOriginChangesAfterAdmission(t *testing.T) {
	for _, op := range []activity.RuntimeOperation{activity.RuntimeUpdateContainer, activity.RuntimeRestoreContainerImage} {
		t.Run(string(op), func(t *testing.T) {
			daemon, reg := newContainerImageHTTP(t)
			f := newRemoteIntegration(t, false, func(agent *runtimeServicesFixture) { configureContainerImageFixture(t, agent, daemon, reg) })
			h, err := activity.NewRuntimeHTTPService(f.hub, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return f.store, nil }, PrepareTarget: func(ctx context.Context, _ string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
				return f.agent.adapter.ResolveRuntimeTarget(ctx, target)
			}, GuardContainer: func(context.Context, string, string) error { return nil }})
			require.NoError(t, err)
			h.SetReady(true)
			command := f.command(op, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
			accepted, err := f.hub.Admit(t.Context(), command)
			require.NoError(t, err)
			daemon.base.mu.Lock()
			old := daemon.base.containers[runtimeContainerID]
			old.Labels["com.docker.compose.version"] = "2"
			daemon.base.containers[runtimeContainerID] = old
			daemon.base.mu.Unlock()
			ref := activity.RuntimeRunReference{Store: f.store, RunID: accepted.ActivityID}
			_, err = f.control.Start(t.Context(), ref, command.Options)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			require.NoError(t, f.control.WaitDispatch(ctx, ref.RunID))
			_, err = f.agent.waitLocal(ref.RunID)
			require.NoError(t, err)
			result, err := f.control.Wait(ctx, ref, nil)
			require.NoError(t, err)
			require.NotNil(t, result.ImageLifecycle)
			require.Equal(t, "container.origin-locked", result.ImageLifecycle.Code)
			require.Empty(t, daemon.base.counts())
			require.Equal(t, 0, daemon.pulls)
			require.Equal(t, 0, daemon.tags)
			require.Equal(t, 0, daemon.creates)
		})
	}
}
