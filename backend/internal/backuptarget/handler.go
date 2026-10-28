// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original hub-only target administration and agent metadata routes.

package backuptarget

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
	"gorm.io/gorm"
)

type Handler struct {
	Service        *Service
	Apply          func(context.Context, dto.TargetSet, string) (dto.TargetSetReceipt, error)
	Test           func(context.Context, string, string) (dto.TargetTestOperation, error)
	References     func(context.Context, string) (dto.TargetReferences, error)
	LocalKey       func(context.Context) (string, error)
	AttachLocalKey func(context.Context, string) error
	KeyViewed      func(context.Context, string) error
}

type IDInput struct {
	ID string `path:"id"`
}
type CreateInput struct{ Body dto.TargetInput }
type UpdateInput struct {
	ID   string `path:"id"`
	Body dto.TargetInput
}
type TestInput struct {
	ID   string `path:"id"`
	Body dto.TargetTestRequest
}
type HostTargetInput struct {
	EnvironmentID string `path:"id"`
	TargetID      string `path:"targetId"`
}
type HostSyncInput struct {
	EnvironmentID string `path:"id"`
	Token         string `header:"X-Arcane-Agent-Token"`
	APIKey        string `header:"X-Api-Key"`
	Body          dto.TargetSet
}
type HostKeyInput struct {
	EnvironmentID string `path:"id"`
}
type HostAttachInput struct {
	EnvironmentID string `path:"id"`
	Body          dto.TargetRecoveryKey
}
type KeyOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         base.ApiResponse[dto.TargetRecoveryKey]
}

func out[T any](data T) *handlerutil.Out[T] {
	return &handlerutil.Out[T]{Body: base.ApiResponse[T]{Success: true, Data: data}}
}
func failure(err error) error {
	var problem *dto.Problem
	if errors.As(err, &problem) {
		return problem
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return huma.Error404NotFound("Backup target not found")
	}
	return dto.NewProblem(502, "backup.target.sync-failed", err)
}
func (h *Handler) List(ctx context.Context, _ *struct{}) (*handlerutil.Out[[]dto.Target], error) {
	rows, err := h.Service.List(ctx)
	if err != nil {
		return nil, failure(err)
	}
	return out(rows), nil
}
func (h *Handler) Get(ctx context.Context, in *IDInput) (*handlerutil.Out[dto.Target], error) {
	row, err := h.Service.Get(ctx, in.ID)
	if err != nil {
		return nil, failure(err)
	}
	return out(row), nil
}
func (h *Handler) Create(ctx context.Context, in *CreateInput) (*handlerutil.Out[dto.Target], error) {
	row, err := h.Service.Create(ctx, in.Body)
	if err != nil {
		return nil, failure(err)
	}
	return out(project(row, nil)), nil
}
func (h *Handler) Update(ctx context.Context, in *UpdateInput) (*handlerutil.Out[dto.Target], error) {
	row, err := h.Service.Update(ctx, in.ID, in.Body)
	if err != nil {
		return nil, failure(err)
	}
	return out(project(row, nil)), nil
}
func (h *Handler) Delete(ctx context.Context, in *IDInput) (*handlerutil.Out[base.MessageResponse], error) {
	if err := h.Service.Retire(ctx, in.ID); err != nil {
		return nil, failure(err)
	}
	return out(base.MessageResponse{Message: "Backup target retired"}), nil
}
func (h *Handler) TestTarget(ctx context.Context, in *TestInput) (*handlerutil.Out[dto.TargetTestOperation], error) {
	result, err := h.Test(ctx, in.ID, in.Body.HostID)
	if err != nil {
		return nil, failure(err)
	}
	return out(result), nil
}
func (h *Handler) Key(ctx context.Context, in *IDInput) (*KeyOutput, error) {
	key, err := h.Service.RecoveryKey(ctx, in.ID)
	if err != nil {
		return nil, failure(err)
	}
	return h.keyOutput(ctx, in.ID, key)
}
func (h *Handler) keyOutput(ctx context.Context, id, key string) (*KeyOutput, error) {
	if h.KeyViewed == nil {
		return nil, huma.Error503ServiceUnavailable("Backup target audit unavailable")
	}
	if err := h.KeyViewed(ctx, id); err != nil {
		return nil, failure(err)
	}
	return &KeyOutput{CacheControl: "no-store", Body: base.ApiResponse[dto.TargetRecoveryKey]{Success: true, Data: dto.TargetRecoveryKey{RecoveryKey: key}}}, nil
}
func (h *Handler) Sync(ctx context.Context, in *HostSyncInput) (*handlerutil.Out[dto.TargetSetReceipt], error) {
	token := in.Token
	if token == "" {
		token = in.APIKey
	}
	result, err := h.Apply(ctx, in.Body, token)
	if err != nil {
		return nil, failure(err)
	}
	return out(result), nil
}
func (h *Handler) HostTest(ctx context.Context, in *HostTargetInput) (*handlerutil.Out[dto.TargetTestOperation], error) {
	result, err := h.Test(ctx, in.TargetID, in.EnvironmentID)
	if err != nil {
		return nil, failure(err)
	}
	return out(result), nil
}
func (h *Handler) HostReferences(ctx context.Context, in *HostTargetInput) (*handlerutil.Out[dto.TargetReferences], error) {
	result, err := h.References(ctx, in.TargetID)
	if err != nil {
		return nil, failure(err)
	}
	return out(result), nil
}
func (h *Handler) HostKey(ctx context.Context, _ *HostKeyInput) (*KeyOutput, error) {
	key, err := h.LocalKey(ctx)
	if err != nil {
		return nil, failure(err)
	}
	return h.keyOutput(ctx, "local", key)
}
func (h *Handler) HostAttach(ctx context.Context, in *HostAttachInput) (*handlerutil.Out[base.MessageResponse], error) {
	if err := h.AttachLocalKey(ctx, in.Body.RecoveryKey); err != nil {
		return nil, failure(err)
	}
	return out(base.MessageResponse{Message: "Recovery key attached"}), nil
}

func RegisterHubRoutes(api huma.API, h *Handler) {
	api = BudgetAPI(api)
	defer RegisterProblemSchemas(api)
	admin := middleware.RequireGlobalAdmin(api)
	op := func(id, method, path string) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: path, Tags: []string{"Backup Targets"}, Middlewares: admin}
	}
	middleware.RegisterWithPermission(api, op("list-backup-targets", http.MethodGet, "/backup-targets"), authz.PermSystemBackupsRead, h.List)
	middleware.RegisterWithPermission(api, op("create-backup-target", http.MethodPost, "/backup-targets"), authz.PermSystemBackupsManage, h.Create)
	middleware.RegisterWithPermission(api, op("get-backup-target", http.MethodGet, "/backup-targets/{id}"), authz.PermSystemBackupsRead, h.Get)
	middleware.RegisterWithPermission(api, op("update-backup-target", http.MethodPut, "/backup-targets/{id}"), authz.PermSystemBackupsManage, h.Update)
	middleware.RegisterWithPermission(api, op("retire-backup-target", http.MethodDelete, "/backup-targets/{id}"), authz.PermSystemBackupsManage, h.Delete)
	test := op("test-backup-target", http.MethodPost, "/backup-targets/{id}/test")
	test.DefaultStatus = 202
	middleware.RegisterWithPermission(api, test, authz.PermSystemBackupsManage, h.TestTarget)
	middleware.RegisterWithPermission(api, op("get-backup-target-recovery-key", http.MethodGet, "/backup-targets/{id}/recovery-key"), authz.PermSystemBackupsRestore, h.Key)
}

// RegisterHostRoutes is called by the allowlisted volume module in all modes;
// the metadata receiver exists only on agents and is never a hub client route.
func RegisterHostRoutes(api huma.API, h *Handler, agent bool) {
	api = BudgetAPI(api)
	defer RegisterProblemSchemas(api)
	prefix := "/environments/{id}/volumes/backups/targets"
	op := func(id, method, path string) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: path, Tags: []string{"Volume Backup"}}
	}
	if agent {
		middleware.RegisterWithPermission(api, op("sync-volume-backup-targets", http.MethodPut, prefix), authz.PermVolumesBackup, h.Sync)
	}
	test := op("test-host-backup-target", http.MethodPost, prefix+"/{targetId}/test")
	test.DefaultStatus = 202
	middleware.RegisterWithPermission(api, test, authz.PermVolumesBackup, h.HostTest)
	middleware.RegisterWithPermission(api, op("get-volume-backup-target-references", http.MethodGet, prefix+"/{targetId}/references"), authz.PermVolumesRead, h.HostReferences)
	keyPerm, attachPerm := authz.PermSystemBackupsRestore, authz.PermSystemBackupsRestore
	if agent {
		keyPerm, attachPerm = authz.PermVolumesBackup, authz.PermVolumesBackup
	}
	middleware.RegisterWithPermission(api, op("get-local-backup-recovery-key", http.MethodGet, prefix+"/local/recovery-key"), keyPerm, h.HostKey)
	middleware.RegisterWithPermission(api, op("attach-local-backup-recovery-key", http.MethodPut, prefix+"/local/recovery-key"), attachPerm, h.HostAttach)
}
