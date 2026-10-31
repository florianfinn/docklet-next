// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: One production runtime graph using existing Docker, registration and transport ports.

package di

import (
	"context"
	"errors"
	"net/http"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	containerDTO "github.com/florianfinn/docklet-next/types/container"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	"go.uber.org/fx"
	"net/url"
)

type runtimeResult struct {
	fx.Out
	Service  *activity.RuntimeService
	HTTP     *activity.RuntimeHTTPService
	Receiver *activity.RuntimeReceiver
	Remote   *activity.RuntimeRemoteCoordinator
}

func provideRuntimeInternal(ctx context.Context, lc fx.Lifecycle, service *activity.ActivityService, projects *project.ProjectService, containers *container.ContainerService,
	environments *environment.EnvironmentService, settings *settings.SettingsService, cfg *config.Config, client *http.Client, tunnels *edge.TunnelRegistry, db *database.DB) (runtimeResult, error) {
	runtimeCtx, cancelRuntime := context.WithCancel(ctx)
	constructed := false
	defer func() {
		if !constructed {
			cancelRuntime()
		}
	}()

	adapter := activity.NewRuntimeServiceAdapter(projects, containers)
	var runtime *activity.RuntimeService
	newSender := func(ctx context.Context, id string) (*activity.RuntimeSender, error) {
		env, err := environments.GetEnvironmentByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if env.AccessToken == nil {
			return nil, environment.ErrInvalidEnvironmentAccessToken
		}
		info, err := environments.CheckAgentCompatibility(ctx, id)
		if err != nil {
			return nil, err
		}
		return activity.NewRuntimeSender(activity.RuntimeRemoteTarget{EnvironmentID: id, Endpoint: env.ApiUrl, Token: *env.AccessToken, Edge: env.IsEdge, AgentContractVersion: info.AgentContractVersion}, environments.AgentAccess, client, tunnels)
	}
	remoteIdentity := func(ctx context.Context, id string) (*activity.RuntimeSender, activitytypes.RuntimeExecutorIdentity, error) {
		sender, err := newSender(ctx, id)
		if err != nil {
			return nil, activitytypes.RuntimeExecutorIdentity{}, err
		}
		identity, err := sender.Register(ctx, activitytypes.RuntimeRegistration{ManagerID: settings.GetSettingsConfig().InstanceID.Value, RegistrationID: id})
		return sender, identity, err
	}
	resolve := func(ctx context.Context, id string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
		if id == "0" {
			return adapter.ResolveRuntimeTarget(ctx, target)
		}
		sender, identity, err := remoteIdentity(ctx, id)
		if err != nil {
			return activity.RuntimeResolvedTarget{}, err
		}
		metadata, err := sender.Resolve(ctx, activitytypes.RuntimeTargetRequest{Registration: identity, Target: target})
		return activity.RuntimeResolvedTarget{Target: metadata.Target, ComposeManaged: metadata.ComposeManaged, Origin: metadata.Origin, ComposeProjectLabel: metadata.ComposeProjectLabel, Protected: metadata.Protected}, err
	}
	runtime = activity.NewRuntimeService(service, activity.RuntimeResolverFunc(func(ctx context.Context, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
		return resolve(ctx, activity.RuntimeEnvironmentFromContext(ctx), target)
	}))
	generation, err := runtime.InitializeGeneration(ctx)
	if err != nil {
		return runtimeResult{}, err
	}
	containers.ConfigureOperations(db.DB, settings.GetSettingsConfig().InstanceID.Value, !cfg.AgentMode && !cfg.EdgeAgent, func(ctx context.Context, environment, id string) (origin.Identity, error) {
		var response base.ApiResponse[containerDTO.Details]
		err := environments.ProxyJSONRequest(ctx, environment, http.MethodGet, "/api/environments/0/containers/"+url.PathEscape(id), nil, &response)
		if err != nil {
			return origin.Identity{}, err
		}
		if !response.Success || response.Data.ID == "" {
			return origin.Identity{}, activity.ErrRuntimeTransport
		}
		return origin.Identity{ID: response.Data.ID, Name: response.Data.Name, Labels: response.Data.Labels, Protected: response.Data.RedeployDisabled}, nil
	}, environments.ReadSnapshots().Forget)
	local, err := runtime.LocalBinding(ctx, generation, settings.GetSettingsConfig().InstanceID.Value)
	if err != nil {
		return runtimeResult{}, err
	}
	if err := runtime.ConfigureLocal(activity.RuntimeLocalConfig{ApplicationContext: runtimeCtx, Store: local, Executor: adapter}); err != nil {
		return runtimeResult{}, err
	}
	receiver, err := activity.NewRuntimeReceiver(runtime, activity.RuntimeResolverFunc(adapter.PrepareRuntimeTarget))
	if err != nil {
		return runtimeResult{}, err
	}
	remote, err := activity.NewRuntimeRemoteCoordinator(runtime, runtimeCtx, func(ctx context.Context, store activity.RuntimeStoreBinding) (activity.RuntimeRemoteTransport, error) {
		return newSender(ctx, store.EnvironmentID)
	}, activity.RuntimeReconcileSchedule{})
	if err != nil {
		return runtimeResult{}, err
	}
	httpService, err := activity.NewRuntimeHTTPService(runtime, activity.RuntimeHTTPPorts{ApplicationContext: runtimeCtx, Agent: cfg.AgentMode, GuardContainer: containers.GuardRuntimeOrigin, InvalidateInventory: environments.ReadSnapshots().Forget,
		PrepareLifecycle: func(ctx context.Context, id string, target activity.RuntimeTarget, store activity.RuntimeStoreBinding, request updaterDTO.LifecycleRequest) (*updaterDTO.LifecycleIntent, error) {
			if id == "0" {
				return adapter.PrepareImageLifecycle(ctx, target.ID, store.RegistrationID, request)
			}
			sender, identity, err := remoteIdentity(ctx, id)
			if err != nil {
				return nil, err
			}
			metadata, err := sender.Resolve(ctx, activitytypes.RuntimeTargetRequest{Registration: identity, Target: target, Lifecycle: &request})
			return metadata.Lifecycle, err
		}, Store: func(ctx context.Context, id string) (activity.RuntimeStoreBinding, error) {
			if id == "0" {
				return runtime.LocalStore(), nil
			}
			_, identity, err := remoteIdentity(ctx, id)
			if err != nil {
				return activity.RuntimeStoreBinding{}, err
			}
			return activity.RuntimeStoreBinding{StoreGeneration: runtime.LocalStore().StoreGeneration, EnvironmentID: id, ManagerID: identity.ManagerID, RegistrationID: identity.RegistrationID, ExecutorGeneration: identity.ExecutorGeneration}, nil
		}, PrepareTarget: func(ctx context.Context, id string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
			if id == "0" {
				return adapter.PrepareRuntimeTarget(ctx, target)
			}
			return resolve(ctx, id, target)
		}})
	if err != nil {
		return runtimeResult{}, err
	}
	environments.SetRuntimeRemoval(runtime.RemoveEnvironment)
	var maintenanceCancel context.CancelFunc
	var maintenanceDone chan error
	lc.Append(fx.Hook{OnStart: func(start context.Context) error {
		if err := runtime.PruneRuntime(start); err != nil {
			return err
		}
		if err := runtime.InitializeLocalStartup(start, runtime.LocalStore()); err != nil {
			return err
		}
		if err := remote.Startup(start); err != nil {
			return err
		}
		maintenanceCtx, cancel := context.WithCancel(runtimeCtx)
		maintenanceCancel, maintenanceDone = cancel, make(chan error, 1)
		go func() { maintenanceDone <- runtime.RunMaintenance(maintenanceCtx) }()
		httpService.SetReady(true)
		return nil
	}, OnStop: func(stop context.Context) error {
		httpService.SetReady(false)
		cancelRuntime()
		if maintenanceCancel != nil {
			maintenanceCancel()
		}
		var maintenanceErr error
		if maintenanceDone != nil {
			select {
			case maintenanceErr = <-maintenanceDone:
			case <-stop.Done():
				return stop.Err()
			}
		}
		return errors.Join(maintenanceErr, remote.Close(stop), runtime.JoinLocal(stop))
	}})
	constructed = true
	return runtimeResult{Service: runtime, HTTP: httpService, Receiver: receiver, Remote: remote}, nil
}
