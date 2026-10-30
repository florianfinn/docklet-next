// Source: docklet-next/issues/314
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original hub-owned origin API and reading-host effect guards.

package container

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
	dto "github.com/florianfinn/docklet-next/types/container"
	dockercontainer "github.com/moby/moby/api/types/container"
	"go.getarcane.app/sys/cgroup"
	"go.getarcane.app/updater/labels"
	"gorm.io/gorm"
)

type containerOperations struct {
	db      *gorm.DB
	origins *origin.Store
	read    func(context.Context, string, string) (origin.Identity, error)
}

// ConfigureOperations is called once by the runtime graph before readiness.
func (s *ContainerService) ConfigureOperations(db *gorm.DB, registration string, hub bool, read func(context.Context, string, string) (origin.Identity, error), invalidate func(string)) {
	s.operations = &containerOperations{db: db, read: read}
	if hub {
		s.operations.origins = &origin.Store{DB: db, LocalRegistration: registration, Invalidate: invalidate}
	}
}
func effectOriginGuard(inspect dockercontainer.InspectResponse) error {
	if inspect.Config == nil {
		return &dto.Problem{Code: "container.origin-locked", Status: 409, Origin: dto.OriginUnknown}
	}
	current, err := cgroup.CurrentContainerID()
	_, compose := inspect.Config.Labels["com.docker.compose.project"]
	_, unraid := inspect.Config.Labels["net.unraid.docker.managed"]
	if compose || unraid || labels.ShouldDisableArcaneServerRedeploy(inspect.Config.Labels, inspect.ID, current, err) {
		return &dto.Problem{Code: "container.origin-locked", Status: 409, Origin: origin.Detect(inspect.Config.Labels).Origin}
	}
	return nil
}
func (s *ContainerService) OverlayInventory(ctx context.Context, environment string, body []byte) ([]byte, error) {
	if s == nil || s.operations == nil || s.operations.origins == nil {
		return body, nil
	}
	return s.operations.origins.Overlay(ctx, environment, body)
}
func (s *ContainerService) overlayLocal(ctx context.Context, value any) error {
	if s.operations == nil || s.operations.origins == nil {
		return nil
	}
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	body, err = s.OverlayInventory(ctx, "0", body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, value)
}
func (s *ContainerService) OriginGuard(ctx context.Context, environment string, id origin.Identity) error {
	if s.operations == nil || s.operations.origins == nil {
		return &dto.Problem{Code: "container.origin-locked", Origin: dto.OriginUnknown, Status: 409}
	}
	return s.operations.origins.Guard(ctx, s.operations.origins.Registration(environment), id)
}
func (s *ContainerService) readOriginIdentity(ctx context.Context, environment, id string) (origin.Identity, error) {
	if environment != "0" && s.operations != nil && s.operations.read != nil {
		return s.operations.read(ctx, environment, id)
	}
	inspect, err := s.GetContainerByID(ctx, id)
	if err != nil {
		return origin.Identity{}, err
	}
	if inspect == nil || inspect.Config == nil {
		return origin.Identity{}, huma.Error503ServiceUnavailable("Container identity unavailable")
	}
	return origin.Identity{ID: inspect.ID, Name: strings.TrimPrefix(inspect.Name, "/"), Labels: inspect.Config.Labels}, nil
}

type originInput struct {
	EnvironmentID string `path:"id"`
	ContainerID   string `path:"containerId"`
	Body          dto.OriginAssignment
}
type originResetInput struct {
	EnvironmentID string `path:"id"`
	ContainerID   string `path:"containerId"`
}
type originOutput struct{ Body dto.OriginFields }

func RegisterOrigins(api huma.API, s *ContainerService) {
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "assign-container-origin", Method: http.MethodPut, Path: "/environments/{id}/containers/{containerId}/origin", Tags: []string{"Containers"}}, authz.PermContainersEdit, func(ctx context.Context, in *originInput) (*originOutput, error) {
		if s == nil || s.operations == nil || s.operations.origins == nil {
			return nil, huma.Error503ServiceUnavailable("Origin storage unavailable")
		}
		actor, ok := userctx.CurrentUserFromContext(ctx)
		if !ok || actor == nil {
			return nil, huma.Error403Forbidden("Authenticated actor required")
		}
		if err := s.operations.origins.AssignFresh(ctx, in.EnvironmentID, in.ContainerID, in.Body, actor.ID, s.readOriginIdentity); err != nil {
			return nil, err
		}
		id, err := s.readOriginIdentity(ctx, in.EnvironmentID, in.ContainerID)
		if err != nil {
			return nil, err
		}
		fields, err := s.operations.origins.Effective(ctx, s.operations.origins.Registration(in.EnvironmentID), id)
		return &originOutput{Body: fields}, err
	})
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "reset-container-origin", Method: http.MethodDelete, Path: "/environments/{id}/containers/{containerId}/origin", Tags: []string{"Containers"}}, authz.PermContainersEdit, func(ctx context.Context, in *originResetInput) (*originOutput, error) {
		if s == nil || s.operations == nil || s.operations.origins == nil {
			return nil, huma.Error503ServiceUnavailable("Origin storage unavailable")
		}
		id, err := s.readOriginIdentity(ctx, in.EnvironmentID, in.ContainerID)
		if err != nil {
			return nil, err
		}
		if err := s.operations.origins.Reset(ctx, in.EnvironmentID, id); err != nil {
			return nil, err
		}
		return &originOutput{Body: origin.Detect(id.Labels)}, nil
	})
}

// GuardRuntimeOrigin reads the executing host directly; retained inventory never enters admission.
func (s *ContainerService) GuardRuntimeOrigin(ctx context.Context, environment, id string) error {
	identity, err := s.readOriginIdentity(ctx, environment, id)
	if err != nil {
		return err
	}
	if environment == "0" {
		inspect, err := s.GetContainerByID(ctx, id)
		if err != nil {
			return err
		}
		if inspect != nil && inspect.Config != nil {
			current, currentErr := cgroup.CurrentContainerID()
			identity.Protected = labels.ShouldDisableArcaneServerRedeploy(inspect.Config.Labels, inspect.ID, current, currentErr)
		}
	}
	return s.OriginGuard(ctx, environment, identity)
}
