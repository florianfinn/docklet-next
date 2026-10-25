// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Legacy and stack image routes share durable admission and application-owned execution.

package activity

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
)

// RuntimeHTTPPorts are trusted construction ports; no browser execution envelope
// can select an executor, registration, storage generation or actor.
type RuntimeHTTPPorts struct {
	ApplicationContext context.Context
	Agent              bool
	Store              func(context.Context, string) (RuntimeStoreBinding, error)
	PrepareLifecycle   func(context.Context, string, RuntimeTarget, RuntimeStoreBinding, updaterDTO.LifecycleRequest) (*updaterDTO.LifecycleIntent, error)
	PrepareTarget      func(context.Context, string, RuntimeTarget) (RuntimeResolvedTarget, error)
}

type RuntimeHTTPService struct {
	service *RuntimeService
	ports   RuntimeHTTPPorts
	ready   atomic.Bool
}

func NewRuntimeHTTPService(service *RuntimeService, ports RuntimeHTTPPorts) (*RuntimeHTTPService, error) {
	if service == nil || ports.ApplicationContext == nil || ports.Store == nil || ports.PrepareTarget == nil {
		return nil, errRuntimeInvalidInternal
	}
	if err := service.BindActivityOwner(); err != nil {
		return nil, err
	}
	h := &RuntimeHTTPService{service: service, ports: ports}
	service.activity.runtimeHTTP = h
	return h, nil
}

// SetReady is owned by startup/restore lifecycle after generation and receipt
// repair. Admission stays closed while the database/process boundary changes.
func (h *RuntimeHTTPService) SetReady(ready bool) { h.ready.Store(ready) }

func runtimePermissionInternal(operation RuntimeOperation) string {
	switch operation {
	case RuntimeUpdateProject, RuntimeRestoreProject:
		return authz.PermProjectsDeploy
	case RuntimeDeployProject, RuntimeRedeployProject:
		return authz.PermProjectsDeploy
	case RuntimeDownProject:
		return authz.PermProjectsDown
	case RuntimeRestartProject:
		return authz.PermProjectsRestart
	case RuntimeStartContainer:
		return authz.PermContainersStart
	case RuntimeStopContainer:
		return authz.PermContainersStop
	case RuntimeRestartContainer:
		return authz.PermContainersRestart
	}
	return ""
}

func (h *RuntimeHTTPService) admit(ctx context.Context, environmentID, key string, operation RuntimeOperation, target RuntimeTarget, options RuntimeOptions) (RuntimeAdmissionResult, RuntimeRunReference, error) {
	ctx, cancelAdmission := context.WithTimeout(ctx, RuntimeAdmissionLimit)
	defer cancelAdmission()
	empty, noRef := RuntimeAdmissionResult{}, RuntimeRunReference{}
	if h == nil || !h.ready.Load() {
		return empty, noRef, huma.Error503ServiceUnavailable("Runtime is not ready")
	}
	if h.ports.Agent {
		return empty, noRef, huma.Error409Conflict("Hub runtime admission required")
	}
	ps, _ := middleware.PermissionsFromContext(ctx)
	if !ps.Allows(runtimePermissionInternal(operation), environmentID) || !ps.Allows(authz.PermActivitiesRead, environmentID) {
		return empty, noRef, huma.Error403Forbidden("Runtime operation and activity access required")
	}
	if runtimeImageOperation(operation) {
		for _, right := range []string{authz.PermProjectsUpdate, authz.PermImagesPull, authz.PermImageUpdatesCheck} {
			if !ps.Allows(right, environmentID) {
				return empty, noRef, huma.Error403Forbidden("Stack update grants required")
			}
		}
	}
	actor, ok := userctx.CurrentUserFromContext(ctx)
	if !ok || actor == nil {
		return empty, noRef, huma.Error403Forbidden("Authenticated actor required")
	}
	if runtimeImageOperation(operation) {
		if _, err := uuid.Parse(key); err != nil {
			return empty, noRef, imageDTO.Failure("update.invalid-target", 400)
		}
	} else if key == "" {
		key = uuid.NewString()
	}
	// Validate semantic options before fresh I/O or admission. The placeholder
	// tuple is used only by the pure validator and never persisted or dispatched.
	command := RuntimeCommand{Store: RuntimeStoreBinding{EnvironmentID: environmentID, StoreGeneration: "validation", ExecutorGeneration: "validation", ManagerID: "validation", RegistrationID: "validation"},
		Key: key, Actor: *actor, Operation: operation, Target: target, Options: options}
	var err error
	if runtimeImageOperation(operation) {
		if target.Kind != "stack" || options.Lifecycle == nil || len(options.Lifecycle.Request.ExpectedRevision) != 64 || (operation == RuntimeRestoreProject) != (options.Lifecycle.Request.PointID != "") {
			return empty, noRef, imageDTO.Failure("update.invalid-target", 400)
		}
	} else {
		canonical, e := runtimeCanonicalizeInternal(command)
		if e != nil {
			return empty, noRef, huma.Error400BadRequest("Invalid runtime command or pull policy")
		}
		command.Options = canonical.command.Options
	}
	command, retained, err := h.service.RetainedCommand(ctx, command)
	if err != nil {
		return empty, noRef, err
	}
	if !retained {
		command, retained, err = h.service.HeldCommand(ctx, command)
		if err != nil {
			return empty, noRef, err
		}
	}
	if !retained {
		if err := h.service.refuseRemovedEnvironmentInternal(ctx, environmentID); err != nil {
			return empty, noRef, err
		}
		command.Store, err = h.ports.Store(ctx, environmentID)
		if err != nil {
			return empty, noRef, err
		}
		resolved, err := h.ports.PrepareTarget(ctx, environmentID, target)
		if err != nil {
			return empty, noRef, &RuntimeTargetError{Code: "runtime.target-unavailable", Cause: err}
		}
		if resolved.ComposeManaged && resolved.Target.ParentProjectID == nil {
			return empty, noRef, &RuntimeTargetError{Code: string(activitytypes.CodeRuntimeParentUnresolved)}
		}
		command.Target = resolved.Target
		if runtimeImageOperation(operation) {
			if h.ports.PrepareLifecycle == nil {
				return empty, noRef, huma.Error503ServiceUnavailable("Image lifecycle unavailable")
			}
			command.Options.Lifecycle, err = h.ports.PrepareLifecycle(ctx, environmentID, command.Target, command.Store, command.Options.Lifecycle.Request)
			if err != nil {
				return empty, noRef, err
			}
		}
	}
	result, err := h.service.Admit(ctx, command)
	if err != nil {
		return empty, noRef, err
	}
	ref := RuntimeRunReference{Store: command.Store, RunID: result.ActivityID}
	if environmentID != "0" && h.service.remote != nil {
		// Acceptance has committed. Viewer cancellation cannot strand this new
		// admission or become dispatch authority; Start registers on app lifetime.
		writeCtx, cancel := context.WithTimeout(h.ports.ApplicationContext, ownerWriteTimeout)
		snapshot, startErr := h.service.remote.Start(writeCtx, ref, command.Options)
		cancel()
		if startErr == nil {
			result.Activity = snapshot
		} else {
			// Storage/continuity failures after admission retain identity/exclusion.
			result.Activity.OutcomeState = activitytypes.RuntimeOutcomeUnknown
		}
	}
	return result, ref, nil
}

func (h *RuntimeHTTPService) wait(ctx context.Context, ref RuntimeRunReference, emit func(activitytypes.OutputLine) error) (activitytypes.RuntimeSnapshot, error) {
	if ref.Store.EnvironmentID != "0" {
		if h.service.remote == nil {
			return activitytypes.RuntimeSnapshot{}, ErrRuntimeTransport
		}
		return h.service.remote.Wait(ctx, ref, emit)
	}
	relay := &RuntimeOutputRelay{}
	events, _, unsubscribe := h.service.activity.Subscribe(ref.Store.EnvironmentID)
	defer unsubscribe()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		detail, err := h.service.GetRuntimeDetail(ctx, ref)
		if err != nil {
			return activitytypes.RuntimeSnapshot{}, err
		}
		if emit != nil {
			if err := relay.Relay(detail, emit); err != nil {
				return detail.Activity, err
			}
		}
		if isTerminalActivityStatusInternal(detail.Activity.Status) || detail.Activity.OutcomeState == activitytypes.RuntimeOutcomeUnknown {
			return detail.Activity, nil
		}
		select {
		case <-ctx.Done():
			return detail.Activity, ctx.Err()
		case <-events:
		case <-tick.C:
		}
	}
}

func runtimeWaitFailure(snapshot activitytypes.RuntimeSnapshot, err error) (int, activitytypes.Code) {
	switch {
	case err == nil && snapshot.OutcomeState != activitytypes.RuntimeOutcomeUnknown && snapshot.Status == activitytypes.StatusFailed && snapshot.ImageLifecycle != nil && snapshot.ImageLifecycle.Code != "":
		return snapshot.ImageLifecycle.HTTPStatus, activitytypes.Code(snapshot.ImageLifecycle.Code)
	case errors.Is(err, context.DeadlineExceeded):
		return 504, activitytypes.CodeRuntimeWaitTimeout
	case errors.Is(err, context.Canceled):
		return 408, activitytypes.CodeRuntimeWaiterDisconnected
	case err != nil || snapshot.OutcomeState == activitytypes.RuntimeOutcomeUnknown:
		return 503, activitytypes.CodeRuntimeOutcomeUnknown
	case snapshot.Status == activitytypes.StatusSuccess:
		return 200, activitytypes.CodeActivitySuccess
	case snapshot.Status == activitytypes.StatusCancelled:
		return 409, activitytypes.CodeRuntimeCancelled
	case snapshot.MessageCode == string(activitytypes.CodeRuntimeInterrupted):
		return 409, activitytypes.CodeRuntimeInterrupted
	default:
		return 500, activitytypes.CodeRuntimeFailed
	}
}
