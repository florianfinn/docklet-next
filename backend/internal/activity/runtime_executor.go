// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Runtime service adapter with frozen stack image intent and fresh target preflight.

package activity

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	"go.getarcane.app/sys/cgroup"
	"go.getarcane.app/updater/labels"
	"io"
	"strings"

	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	mobycontainer "github.com/moby/moby/api/types/container"
	dockertypes "go.getarcane.app/docker/types"
)

type runtimeProjectPort interface {
	ResolveRuntimeProjectIdentity(context.Context, string, string) (projecttypes.Reference, bool, error)
	DeployProject(context.Context, string, usertypes.Actor, *projecttypes.DeployOptions) error
	DownProject(context.Context, string, usertypes.Actor) error
	RestartProject(context.Context, string, []string, usertypes.Actor) error
	RedeployProject(context.Context, string, usertypes.Actor, *projecttypes.DeployOptions) error
}

type runtimeContainerPort interface {
	GetContainerByID(context.Context, string) (*mobycontainer.InspectResponse, error)
	StartContainer(context.Context, string, usertypes.Actor) error
	StopContainer(context.Context, string, usertypes.Actor) error
	RestartContainer(context.Context, string, usertypes.Actor) error
}

// RuntimeServiceAdapter is constructed only by trusted local runtime setup.
// Its inputs have already passed operation and activity-read grants and carry
// canonical raw IDs; aliases and host-qualified IDs are resolved by F-d.
type RuntimeServiceAdapter struct {
	projects   runtimeProjectPort
	containers runtimeContainerPort
}

func NewRuntimeServiceAdapter(projects runtimeProjectPort, containers runtimeContainerPort) *RuntimeServiceAdapter {
	return &RuntimeServiceAdapter{projects: projects, containers: containers}
}

var _ RuntimeExecutor = (*RuntimeServiceAdapter)(nil)
var _ RuntimeTargetResolver = (*RuntimeServiceAdapter)(nil)

func (a *RuntimeServiceAdapter) ResolveRuntimeTarget(ctx context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
	if a == nil {
		return RuntimeResolvedTarget{}, errors.New("runtime services unavailable")
	}
	switch target.Kind {
	case "stack":
		if a.projects == nil || target.ID == "" || target.ParentProjectID != nil {
			return RuntimeResolvedTarget{}, errors.New("invalid runtime stack target")
		}
		ref, found, err := a.projects.ResolveRuntimeProjectIdentity(ctx, target.ID, "")
		if err != nil {
			return RuntimeResolvedTarget{}, err
		}
		if !found || ref.ID != target.ID {
			return RuntimeResolvedTarget{}, errors.New("runtime stack identity unavailable")
		}
		return RuntimeResolvedTarget{Target: RuntimeTarget{Kind: "stack", ID: ref.ID, Name: &ref.Name}}, nil
	case "container":
		if a.containers == nil || a.projects == nil || !runtimeFullContainerID(target.ID) {
			return RuntimeResolvedTarget{}, errors.New("invalid canonical container ID")
		}
		inspect, err := a.containers.GetContainerByID(ctx, target.ID)
		if err != nil {
			return RuntimeResolvedTarget{}, err
		}
		if inspect == nil || inspect.ID != target.ID || inspect.Config == nil {
			return RuntimeResolvedTarget{}, errors.New("container inspection identity unavailable")
		}
		fields := origin.Detect(inspect.Config.Labels)
		currentID, currentErr := cgroup.CurrentContainerID()
		_, hasProject := inspect.Config.Labels["com.docker.compose.project"]
		resolved := RuntimeResolvedTarget{Target: RuntimeTarget{Kind: "container", ID: inspect.ID}, Origin: &fields, ComposeProjectLabel: hasProject, Protected: labels.ShouldDisableArcaneServerRedeploy(inspect.Config.Labels, inspect.ID, currentID, currentErr)}
		if name := strings.TrimPrefix(inspect.Name, "/"); name != "" {
			resolved.Target.Name = &name
		}
		if composeName, managed := inspect.Config.Labels["com.docker.compose.project"]; managed {
			resolved.ComposeManaged = true
			if composeName != "" {
				ref, found, err := a.projects.ResolveRuntimeProjectIdentity(ctx, "", composeName)
				if err != nil {
					return RuntimeResolvedTarget{}, err
				}
				if found {
					resolved.Target.ParentProjectID = &ref.ID
				}
			}
		}
		return resolved, nil
	default:
		return RuntimeResolvedTarget{}, errors.New("unsupported runtime target kind")
	}
}

func runtimeFullContainerID(id string) bool {
	if len(id) != 64 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// ExecuteRuntime rechecks identity immediately before dispatch. A changed
// scope cannot be silently executed; Docker/Compose changes after this read
// remain outside the application's atomic control.
func (a *RuntimeServiceAdapter) ExecuteRuntime(ctx context.Context, command RuntimeCommand, output io.Writer) error {
	resolved, err := a.ResolveRuntimeTarget(ctx, command.Target)
	if err != nil {
		return &RuntimeTargetError{Code: "runtime.target-unavailable", Cause: err}
	}
	if resolved.ComposeManaged && resolved.Target.ParentProjectID == nil {
		return &RuntimeTargetError{Code: "runtime.parent-unresolved"}
	}
	if resolved.Target.Kind != command.Target.Kind || resolved.Target.ID != command.Target.ID ||
		!runtimeEqualInternal(resolved.Target.ParentProjectID, command.Target.ParentProjectID) {
		return &RuntimeTargetError{Code: "runtime.target-changed"}
	}
	if command.Target.Kind == "stack" && (command.Target.Name == nil || resolved.Target.Name == nil || *resolved.Target.Name != *command.Target.Name) {
		return &RuntimeTargetError{Code: "runtime.target-changed"}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx = updaterDTO.WithExecutionIdentity(ctx, updaterDTO.ExecutionIdentity{RunID: command.ProposedRunID, RegistrationID: command.Store.RegistrationID})
	ctx = context.WithValue(ctx, dockertypes.ProgressWriterKey{}, output)
	switch command.Operation {
	case RuntimeUpdateProject, RuntimeRestoreProject:
		port, ok := a.projects.(runtimeImageLifecyclePort)
		if !ok {
			return &RuntimeTargetError{Code: "update.invalid-target"}
		}
		_, err := port.ExecuteImageLifecycle(ctx, command.Target.ID, command.ProposedRunID, command.Store.RegistrationID, command.Options.Lifecycle, command.Actor)
		return err
	case RuntimeDeployProject:
		return a.projects.DeployProject(ctx, command.Target.ID, command.Actor, command.Options.Deploy)
	case RuntimeDownProject:
		return a.projects.DownProject(ctx, command.Target.ID, command.Actor)
	case RuntimeRestartProject:
		return a.projects.RestartProject(ctx, command.Target.ID, command.Options.RestartServices, command.Actor)
	case RuntimeRedeployProject:
		return a.projects.RedeployProject(ctx, command.Target.ID, command.Actor, command.Options.Deploy)
	case RuntimeStartContainer:
		return a.containers.StartContainer(ctx, command.Target.ID, command.Actor)
	case RuntimeStopContainer:
		return a.containers.StopContainer(ctx, command.Target.ID, command.Actor)
	case RuntimeRestartContainer:
		return a.containers.RestartContainer(ctx, command.Target.ID, command.Actor)
	default:
		return &RuntimeTargetError{Code: "runtime.target-unavailable", Cause: errors.New("unsupported runtime operation")}
	}
}

type runtimeImageLifecyclePort interface {
	PrepareImageLifecycle(context.Context, string, string, updaterDTO.LifecycleRequest) (*updaterDTO.LifecycleIntent, error)
	ExecuteImageLifecycle(context.Context, string, string, string, *updaterDTO.LifecycleIntent, usertypes.Actor) (*updaterDTO.LifecycleResult, error)
	ReconcileImageLifecycle(context.Context) error
}

func (a *RuntimeServiceAdapter) PrepareImageLifecycle(ctx context.Context, id, registration string, request updaterDTO.LifecycleRequest) (*updaterDTO.LifecycleIntent, error) {
	port, ok := a.projects.(runtimeImageLifecyclePort)
	if !ok {
		return nil, &RuntimeTargetError{Code: "update.invalid-target"}
	}
	return port.PrepareImageLifecycle(ctx, id, registration, request)
}
func (a *RuntimeServiceAdapter) RuntimeSettled(ctx context.Context) {
	if port, ok := a.projects.(runtimeImageLifecyclePort); ok {
		_ = port.ReconcileImageLifecycle(ctx)
	}
}
