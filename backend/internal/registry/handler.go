// Source: getarcaneapp/arcane/backend/internal/registry/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package registry

import (
	"context"
	imageDTO "github.com/florianfinn/docklet-next/types/image"

	"log/slog"
	"time"

	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/containerregistry"

	"go.getarcane.app/sys/crypto"

	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

const registryRemoteSyncFailureMessageInternal = "Failed to fan out registry sync to remote environments"

// ContainerRegistryHandler handles container registry management endpoints.
type ContainerRegistryHandler struct {
	registryService      *ContainerRegistryService
	syncRemoteRegistries func(context.Context) error
}

type ListContainerRegistriesInput struct {
	Search string `query:"search" doc:"Search query"`
	Sort   string `query:"sort" doc:"Column to sort by"`
	Order  string `query:"order" default:"asc" doc:"Sort direction"`
	Start  int    `query:"start" default:"0" doc:"Start index"`
	Limit  int    `query:"limit" default:"20" doc:"Items per page"`
}

type CreateContainerRegistryInput struct {
	Body containerregistry.CreateContainerRegistryRequest
}

type GetContainerRegistryInput struct {
	ID string `path:"id" doc:"Registry ID"`
}

type UpdateContainerRegistryInput struct {
	ID   string `path:"id" doc:"Registry ID"`
	Body containerregistry.UpdateContainerRegistryRequest
}

type DeleteContainerRegistryInput struct {
	ID string `path:"id" doc:"Registry ID"`
}

type TestContainerRegistryInput struct {
	ID string `path:"id" doc:"Registry ID"`
}

type SyncContainerRegistriesInput struct {
	Body containerregistry.SyncRequest
}

// NewHandler builds the container registry HTTP handler.
func NewHandler(registryService *ContainerRegistryService, syncRemoteRegistries func(context.Context) error) *ContainerRegistryHandler {
	return &ContainerRegistryHandler{registryService: registryService, syncRemoteRegistries: syncRemoteRegistries}
}

// ListRegistries returns a paginated list of container registries.
func (h *ContainerRegistryHandler) ListRegistries(ctx context.Context, input *ListContainerRegistriesInput) (*handlerutil.Page[containerregistry.ContainerRegistry], error) {
	params := handlerutil.PaginationParams(input.Start, input.Limit, input.Sort, input.Order, input.Search)

	registries, paginationResp, err := h.registryService.GetRegistriesPaginated(ctx, params)
	if err != nil {
		return nil, registryHTTPError(err)
	}

	return &handlerutil.Page[containerregistry.ContainerRegistry]{
		Body: base.Paginated[containerregistry.ContainerRegistry]{
			Success:    true,
			Data:       registries,
			Pagination: handlerutil.PaginationResponse(paginationResp),
		},
	}, nil
}

// GetPullUsage returns pull usage visibility for configured registries.
func (h *ContainerRegistryHandler) GetPullUsage(ctx context.Context, input *struct{}) (*handlerutil.Out[containerregistry.PullUsageResponse], error) {
	usage, err := h.registryService.GetRegistryPullUsage(ctx)
	if err != nil {
		return nil, registryHTTPError(err)
	}

	return &handlerutil.Out[containerregistry.PullUsageResponse]{
		Body: base.ApiResponse[containerregistry.PullUsageResponse]{
			Success: true,
			Data:    usage,
		},
	}, nil
}

// CreateRegistry creates a new container registry.
func (h *ContainerRegistryHandler) CreateRegistry(ctx context.Context, input *CreateContainerRegistryInput) (*handlerutil.Out[containerregistry.ContainerRegistry], error) {
	reg, err := h.registryService.CreateRegistry(ctx, input.Body)
	if err != nil {
		return nil, registryHTTPError(err)
	}

	h.triggerRemoteRegistrySync(ctx, "registry creation")

	body := base.ApiResponse[containerregistry.ContainerRegistry]{Success: true, Data: registryProjection(reg)}

	return &handlerutil.Out[containerregistry.ContainerRegistry]{Body: body}, nil
}

// GetRegistry returns a container registry by ID.
func (h *ContainerRegistryHandler) GetRegistry(ctx context.Context, input *GetContainerRegistryInput) (*handlerutil.Out[containerregistry.ContainerRegistry], error) {
	reg, err := h.registryService.GetRegistryByID(ctx, input.ID)
	if err != nil {
		return nil, registryHTTPError(err)
	}

	body := base.ApiResponse[containerregistry.ContainerRegistry]{Success: true, Data: registryProjection(reg)}

	return &handlerutil.Out[containerregistry.ContainerRegistry]{Body: body}, nil
}

// UpdateRegistry updates a container registry.
func (h *ContainerRegistryHandler) UpdateRegistry(ctx context.Context, input *UpdateContainerRegistryInput) (*handlerutil.Out[containerregistry.ContainerRegistry], error) {
	reg, err := h.registryService.UpdateRegistry(ctx, input.ID, input.Body)
	if err != nil {
		return nil, registryHTTPError(err)
	}

	h.triggerRemoteRegistrySync(ctx, "registry update")

	body := base.ApiResponse[containerregistry.ContainerRegistry]{Success: true, Data: registryProjection(reg)}

	return &handlerutil.Out[containerregistry.ContainerRegistry]{Body: body}, nil
}

// DeleteRegistry deletes a container registry.
func (h *ContainerRegistryHandler) DeleteRegistry(ctx context.Context, input *DeleteContainerRegistryInput) (*handlerutil.Out[base.MessageResponse], error) {
	if err := h.registryService.DeleteRegistry(ctx, input.ID); err != nil {
		return nil, registryHTTPError(err)
	}

	h.triggerRemoteRegistrySync(ctx, "registry deletion")

	return &handlerutil.Out[base.MessageResponse]{
		Body: base.ApiResponse[base.MessageResponse]{
			Success: true,
			Data: base.MessageResponse{
				Message: "Container registry deleted successfully",
			},
		},
	}, nil
}

// TestRegistry tests connectivity to a container registry.
func (h *ContainerRegistryHandler) TestRegistry(ctx context.Context, input *TestContainerRegistryInput) (*handlerutil.Out[containerregistry.RegistryTestResult], error) {
	checkedAt := time.Now().UTC()
	failure := func(err error) (*handlerutil.Out[containerregistry.RegistryTestResult], error) {
		problem := imageDTO.Neutral(registryHTTPError(err), "registry.unavailable", 503)
		return nil, &containerregistry.RegistryTestResult{Status: problem.Status, CheckedAt: checkedAt, Code: problem.Problem.Code}
	}
	reg, err := h.registryService.GetRegistryByID(ctx, input.ID)
	if err != nil {
		return failure(err)
	}
	if reg.Token == "" && reg.AWSSecretAccessKey == "" {
		return failure(imageDTO.Failure("registry.authentication-required", 401))
	}
	if err = settings.ValidateCredentialEncryption(); err != nil {
		return failure(err)
	}
	if reg.RegistryType == "ecr" {
		err = h.registryService.TestECRRegistry(ctx, reg)
	} else {
		var plaintext string
		plaintext, err = crypto.Decrypt(reg.Token)
		if err != nil {
			err = imageDTO.Failure("registry.credential-unavailable", 503)
		} else {
			err = h.registryService.TestRegistry(ctx, reg.URL, reg.Username, plaintext)
		}
	}
	if err != nil {
		return failure(registryProbeError(err, false))
	}
	return &handlerutil.Out[containerregistry.RegistryTestResult]{Body: base.ApiResponse[containerregistry.RegistryTestResult]{Success: true, Data: containerregistry.RegistryTestResult{OK: true, CheckedAt: checkedAt}}}, nil
}

// SyncRegistries syncs container registries from a remote source.
func (h *ContainerRegistryHandler) SyncRegistries(ctx context.Context, input *SyncContainerRegistriesInput) (*handlerutil.Out[base.MessageResponse], error) {
	if err := h.registryService.SyncRegistries(ctx, input.Body.Registries); err != nil {
		return nil, registryHTTPError(err)
	}

	return &handlerutil.Out[base.MessageResponse]{
		Body: base.ApiResponse[base.MessageResponse]{
			Success: true,
			Data: base.MessageResponse{
				Message: "Registries synced successfully",
			},
		},
	}, nil
}

func (h *ContainerRegistryHandler) triggerRemoteRegistrySync(ctx context.Context, reason string) {
	if h.syncRemoteRegistries == nil {
		return
	}

	detachedCtx := context.WithoutCancel(ctx)

	go func(syncCtx context.Context, syncReason string) {
		if err := h.syncRemoteRegistries(syncCtx); err != nil {
			slog.WarnContext(syncCtx, registryRemoteSyncFailureMessageInternal, "reason", syncReason, "error", err.Error())
		}
	}(detachedCtx, reason)
}
