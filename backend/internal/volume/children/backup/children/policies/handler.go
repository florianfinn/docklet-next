// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/children/policies/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; M12 backup foundation.

package policies

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/volume"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
)

type Handler struct {
	BeforeLocalUpdate  func(context.Context, *UpdateVolumeBackupPolicyInput) error
	AroundUpdate       func(context.Context, *UpdateVolumeBackupPolicyInput, func() error) error
	service            *Service
	activityService    *activity.ActivityService
	environmentService *environment.EnvironmentService
	appCtx             context.Context
}

type GetVolumeBackupPolicyInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	VolumeName    string `path:"volumeName" doc:"Volume name"`
}

type UpdateVolumeBackupPolicyInput struct {
	EnvironmentID string                      `path:"id" doc:"Environment ID"`
	VolumeName    string                      `path:"volumeName" doc:"Volume name"`
	Body          volume.UpdateBackupPolicies `doc:"Scheduled volume backup policies"`
}

func NewHandler(service *Service, activityService *activity.ActivityService, environmentService *environment.EnvironmentService, appCtx context.Context) *Handler {
	return &Handler{service: service, activityService: activityService, environmentService: environmentService, appCtx: appCtx}
}

func (h *Handler) GetBackupPolicy(ctx context.Context, input *GetVolumeBackupPolicyInput) (*handlerutil.Out[volume.BackupPolicyCollection], error) {
	if input.EnvironmentID != "0" {
		if h.environmentService == nil {
			return nil, huma.Error500InternalServerError("environment service not available")
		}
		remotePath := fmt.Sprintf("/api/environments/0/volumes/%s/backup-policy", url.PathEscape(input.VolumeName))
		response, err := handlerutil.RemoteJSONProxy(
			h.environmentService.ProxyJSONRequest,
		).JSON[base.ApiResponse[volume.BackupPolicyCollection]](
			ctx,
			input.EnvironmentID,
			http.MethodGet,
			remotePath,
			nil,
		)
		if err != nil {
			return nil, err
		}
		return &handlerutil.Out[volume.BackupPolicyCollection]{Body: *response}, nil
	}

	policies, err := h.service.GetBackupPolicies(ctx, input.VolumeName)
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	return &handlerutil.Out[volume.BackupPolicyCollection]{Body: base.ApiResponse[volume.BackupPolicyCollection]{Success: true, Data: *policies}}, nil
}

func (h *Handler) UpdateBackupPolicy(ctx context.Context, input *UpdateVolumeBackupPolicyInput) (*handlerutil.Out[volume.BackupPolicyCollection], error) {
	if h.AroundUpdate != nil {
		cloned := *h
		cloned.AroundUpdate = nil
		var result *handlerutil.Out[volume.BackupPolicyCollection]
		err := h.AroundUpdate(ctx, input, func() error { var err error; result, err = cloned.UpdateBackupPolicy(ctx, input); return err })
		return result, err
	}
	user, _ := userctx.CurrentUserFromContext(ctx)
	var policies *volume.BackupPolicyCollection
	var remoteResponse *base.ApiResponse[volume.BackupPolicyCollection]
	errorStatus := http.StatusBadRequest
	runtimeCtx := utils.ActivityRuntimeContext(ctx, h.appCtx)
	_, err := activitylib.RunHandlerActivity(runtimeCtx, h.activityService, activitylib.HandlerOptions{
		EnvironmentID:  input.EnvironmentID,
		Type:           activitytypes.TypeResourceAction,
		ResourceType:   "volume_backup_policy",
		ResourceID:     input.VolumeName,
		ResourceName:   input.VolumeName,
		User:           user,
		Step:           "Saving backup policies",
		Message:        "Saving volume backup policies",
		SuccessMessage: "Volume backup policies saved successfully",
		Metadata: database.JSON{
			"action":      "update_volume_backup_policy",
			"policyCount": len(input.Body.Policies),
		},
	}, func(activityCtx context.Context) error {
		if input.EnvironmentID != "0" {
			if h.environmentService == nil {
				errorStatus = http.StatusInternalServerError
				return errors.New("environment service not available")
			}
			remotePath := fmt.Sprintf("/api/environments/0/volumes/%s/backup-policy", url.PathEscape(input.VolumeName))
			var proxyErr error
			remoteResponse, proxyErr = handlerutil.RemoteJSONProxy(
				h.environmentService.ProxyJSONRequest,
			).JSON[base.ApiResponse[volume.BackupPolicyCollection]](
				activityCtx,
				input.EnvironmentID,
				http.MethodPut,
				remotePath,
				input.Body,
			)
			if proxyErr != nil {
				errorStatus = http.StatusBadGateway
			}
			return proxyErr
		}
		var updateErr error
		policies, updateErr = h.service.UpdateBackupPoliciesChecked(activityCtx, input.VolumeName, input.Body.Policies, func() error {
			if h.BeforeLocalUpdate != nil {
				return h.BeforeLocalUpdate(activityCtx, input)
			}
			return nil
		})
		return updateErr
	})
	if err != nil {
		var typed huma.StatusError
		if errors.As(err, &typed) {
			return nil, typed
		}
		if errorStatus == http.StatusInternalServerError {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		if errorStatus == http.StatusBadGateway {
			return nil, huma.NewError(http.StatusBadGateway, err.Error())
		}
		return nil, huma.Error400BadRequest(err.Error())
	}
	if remoteResponse != nil {
		return &handlerutil.Out[volume.BackupPolicyCollection]{Body: *remoteResponse}, nil
	}
	return &handlerutil.Out[volume.BackupPolicyCollection]{Body: base.ApiResponse[volume.BackupPolicyCollection]{Success: true, Data: *policies}}, nil
}
