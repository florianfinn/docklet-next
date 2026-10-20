// Source: getarcaneapp/arcane/backend/internal/environment/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed GitOps and saved Git repositories; functional cut: removed S3 backup destinations.

package environment

import (
	"context"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/environment"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/samber/mo"
	"go.getarcane.app/kit/pkg"
	"go.getarcane.app/kit/pkg/mapping"
	"log/slog"
	"net/http"
	"strings"
)

// CreateEnvironment creates a new environment.
func (h *EnvironmentHandler) CreateEnvironment(ctx context.Context, input *CreateEnvironmentInput) (*handlerutil.Out[EnvironmentWithApiKey], error) {
	user, err := handlerutil.RequireUser(ctx)
	if err != nil {
		return nil, err
	}

	env := &Environment{
		ApiUrl:  input.Body.ApiUrl,
		Enabled: true,
	}
	if input.Body.Name != nil {
		env.Name = *input.Body.Name
	}
	if input.Body.Enabled != nil {
		env.Enabled = *input.Body.Enabled
	}
	if input.Body.IsEdge != nil {
		env.IsEdge = *input.Body.IsEdge
	}

	// Determine pairing method
	useApiKey := input.Body.UseApiKey != nil && *input.Body.UseApiKey

	if useApiKey {
		return h.createEnvironmentWithApiKeyInternal(ctx, env, user)
	}

	return h.createEnvironmentLegacyInternal(ctx, env, user, input.Body)
}

func (h *EnvironmentHandler) createEnvironmentWithApiKeyInternal(ctx context.Context, env *Environment, user *usertypes.Actor) (*handlerutil.Out[EnvironmentWithApiKey], error) {
	env.Status = string(EnvironmentStatusPending)

	created, err := h.environmentService.CreateEnvironment(ctx, env, &user.ID, &user.Username)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to create environment: " + err.Error())
	}

	// Generate API key for environment
	apiKeyDto, err := h.apiKeyService.CreateEnvironmentApiKey(ctx, created.ID)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create environment API key", "environmentId", created.ID, "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to create environment API key")
	}

	// Store the full API key in AccessToken for manager-to-agent auth.
	apiKey := apiKeyDto.Key

	// Link the API key to the environment for manager use.
	updates := map[string]any{
		"api_key_id":   apiKeyDto.ID,
		"access_token": apiKey,
	}
	updated, err := h.environmentService.UpdateEnvironment(ctx, created.ID, updates, &user.ID, &user.Username)
	if err != nil {
		// Remove the key so a failed create does not leave an orphaned row
		// behind. ErrApiKeyProtected means the link actually landed and the
		// update failed afterwards, so the key legitimately belongs to the
		// environment and must survive; deleting the environment cascades it.
		if delErr := h.apiKeyService.DeleteApiKey(ctx, apiKeyDto.ID); delErr != nil &&
			!errors.Is(delErr, apikey.ErrApiKeyNotFound) && !errors.Is(delErr, apikey.ErrApiKeyProtected) {
			slog.ErrorContext(ctx, "Failed to clean up unlinked environment API key", "environmentId", created.ID, "error", delErr.Error())
		}
		slog.ErrorContext(ctx, "Failed to link API key to environment", "environmentId", created.ID, "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to link API key")
	}
	created = updated

	out, mapErr := mapping.MapOne[*Environment, environment.Environment](created)
	if mapErr != nil {
		return nil, huma.Error500InternalServerError("Failed to map environment")
	}
	h.applyEdgeRuntimeStateInternal(ctx, &out)

	return &handlerutil.Out[EnvironmentWithApiKey]{
		Body: base.ApiResponse[EnvironmentWithApiKey]{
			Success: true,
			Data: EnvironmentWithApiKey{
				Environment: out,
				ApiKey:      new(apiKeyDto.Key),
			},
		},
	}, nil
}

func (h *EnvironmentHandler) createEnvironmentLegacyInternal(ctx context.Context, env *Environment, user *usertypes.Actor, body environment.Create) (*handlerutil.Out[EnvironmentWithApiKey], error) {
	if env.IsEdge || strings.HasPrefix(strings.ToLower(strings.TrimSpace(env.ApiUrl)), "edge://") {
		return nil, huma.Error400BadRequest("Edge hosts require API key registration (useApiKey=true)")
	}
	if body.AccessToken != nil && *body.AccessToken != "" {
		env.AccessToken = body.AccessToken
	}

	created, err := h.environmentService.CreateEnvironment(ctx, env, &user.ID, &user.Username)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to create environment: " + err.Error())
	}

	// Registry delivery is detached from the request and gated by agent compatibility.
	if created.AccessToken != nil && *created.AccessToken != "" {
		h.triggerEnvironmentResourceSyncInternal(ctx, created.ID, created.Name, "environment creation")
	}

	out, mapErr := mapping.MapOne[*Environment, environment.Environment](created)
	if mapErr != nil {
		return nil, huma.Error500InternalServerError("Failed to map environment")
	}
	h.applyEdgeRuntimeStateInternal(ctx, &out)

	return &handlerutil.Out[EnvironmentWithApiKey]{
		Body: base.ApiResponse[EnvironmentWithApiKey]{
			Success: true,
			Data: EnvironmentWithApiKey{
				Environment: out,
			},
		},
	}, nil
}

// GetEnvironment returns an environment by ID.
func (h *EnvironmentHandler) GetEnvironment(ctx context.Context, input *GetEnvironmentInput) (*handlerutil.Out[environment.Environment], error) {
	env, err := h.environmentService.GetEnvironmentByID(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("Environment not found")
	}

	out, mapErr := mapping.MapOne[*Environment, environment.Environment](env)
	if mapErr != nil {
		return nil, huma.Error500InternalServerError("Failed to map environment")
	}
	h.applyEdgeRuntimeStateInternal(ctx, &out)
	if env.IsEdge {
		if certInfo, certErr := readGeneratedEdgeMTLSCertificateInfoInternal(h.cfg, env.ID); certErr == nil {
			out.EdgeMTLSCertificate = certInfo
		}
	}

	return &handlerutil.Out[environment.Environment]{
		Body: base.ApiResponse[environment.Environment]{
			Success: true,
			Data:    out,
		},
	}, nil
}

// UpdateEnvironment updates an environment.
func (h *EnvironmentHandler) UpdateEnvironment(ctx context.Context, input *UpdateEnvironmentInput) (*handlerutil.Out[environment.Environment], error) {
	isLocalEnv := input.ID == localDockerEnvironmentID
	updates := h.buildUpdateMapInternal(&input.Body, isLocalEnv)

	h.handleEnvironmentPairingInternal(ctx, input.ID, &input.Body, updates, isLocalEnv)

	user, _ := userctx.CurrentUserFromContext(ctx)
	var userID, username *string
	if user != nil {
		userID = new(user.ID)
		username = new(user.Username)
	}
	updated, updateErr := h.environmentService.UpdateEnvironment(ctx, input.ID, updates, userID, username)
	if updateErr != nil {
		apiErr := common.ToAPIError(updateErr)
		if apiErr.HTTPStatus() == http.StatusInternalServerError {
			return nil, huma.Error500InternalServerError("Failed to update environment")
		}
		return nil, huma.NewError(apiErr.HTTPStatus(), apiErr.Message)
	}

	out, mapErr := mapping.MapOne[*Environment, environment.Environment](updated)
	if mapErr != nil {
		return nil, huma.Error500InternalServerError("Failed to map environment")
	}
	h.applyEdgeRuntimeStateInternal(ctx, &out)

	// If regenerating API key, return the new key
	var newApiKey *string
	if input.Body.RegenerateApiKey != nil && *input.Body.RegenerateApiKey {
		localUser, err := handlerutil.RequireUser(ctx)
		if err != nil {
			return nil, err
		}

		apiKey, err := h.environmentService.RegenerateEnvironmentApiKey(ctx, updated, localUser.ID, localUser.Username)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to regenerate API key", "environmentId", input.ID, "error", err.Error())
			return nil, huma.Error500InternalServerError("Failed to regenerate API key")
		}

		// Fetch updated environment
		updated, err = h.environmentService.GetEnvironmentByID(ctx, input.ID)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to fetch updated environment", "environmentId", input.ID, "error", err.Error())
			return nil, huma.Error500InternalServerError("Failed to fetch updated environment")
		}

		// Re-map with updated environment data
		out, mapErr = mapping.MapOne[*Environment, environment.Environment](updated)
		if mapErr != nil {
			return nil, huma.Error500InternalServerError("Failed to map environment")
		}
		h.applyEdgeRuntimeStateInternal(ctx, &out)

		newApiKey = new(apiKey)
	}

	if newApiKey == nil {
		h.triggerPostUpdateTasksInternal(ctx, input.ID, updated, &input.Body)
	}

	// Set the API key on the response if regenerated
	out.ApiKey = newApiKey

	return &handlerutil.Out[environment.Environment]{
		Body: base.ApiResponse[environment.Environment]{
			Success: true,
			Data:    out,
		},
	}, nil
}

func (h *EnvironmentHandler) applyEdgeRuntimeStateInternal(ctx context.Context, env *environment.Environment) {
	h.environmentService.ApplyHostRuntimeState(ctx, env)
}

// DeleteEnvironment deletes an environment.
func (h *EnvironmentHandler) DeleteEnvironment(ctx context.Context, input *DeleteEnvironmentInput) (*handlerutil.Out[base.MessageResponse], error) {
	if input.ID == localDockerEnvironmentID {
		return nil, huma.Error400BadRequest("Cannot delete local environment")
	}

	user, _ := userctx.CurrentUserFromContext(ctx)
	var userID, username *string
	if user != nil {
		userID = new(user.ID)
		username = new(user.Username)
	}
	if err := h.environmentService.DeleteEnvironment(ctx, input.ID, userID, username); err != nil {
		var refusal *common.APIError
		if errors.As(err, &refusal) && refusal.HTTPStatus() == 409 {
			return nil, huma.Error409Conflict(refusal.Message)
		}
		return nil, huma.Error500InternalServerError("Failed to delete environment: " + err.Error())
	}

	return &handlerutil.Out[base.MessageResponse]{
		Body: base.ApiResponse[base.MessageResponse]{
			Success: true,
			Data: base.MessageResponse{
				Message: "Environment deleted successfully",
			},
		},
	}, nil
}

// TestConnection tests connectivity to an environment.
func (h *EnvironmentHandler) TestConnection(ctx context.Context, input *TestConnectionInput) (*handlerutil.Out[environment.Test], error) {
	var apiUrl *string
	if input.Body != nil {
		apiUrl = input.Body.ApiUrl
	}
	if apiUrl != nil {
		permissions, ok := middleware.PermissionsFromContext(ctx)
		if !ok || !permissions.Allows(authz.PermEnvironmentsUpdate, "") {
			return nil, huma.Error403Forbidden("permission denied: " + authz.PermEnvironmentsUpdate)
		}
	}

	status, err := h.environmentService.TestConnection(ctx, input.ID, apiUrl)
	resp := environment.Test{Status: status}
	if err != nil {
		if apiUrl == nil {
			resp.Message = new(err.Error())
		} else {
			apiErr := common.ToAPIError(err)
			err = huma.NewError(apiErr.HTTPStatus(), apiErr.Message)
		}
		return &handlerutil.Out[environment.Test]{
			Body: base.ApiResponse[environment.Test]{
				Success: false,
				Data:    resp,
			},
		}, err
	}

	return &handlerutil.Out[environment.Test]{
		Body: base.ApiResponse[environment.Test]{
			Success: true,
			Data:    resp,
		},
	}, nil
}

// UpdateHeartbeat updates the heartbeat for an environment.
func (h *EnvironmentHandler) UpdateHeartbeat(ctx context.Context, input *UpdateHeartbeatInput) (*handlerutil.Out[base.MessageResponse], error) {
	if err := h.environmentService.UpdateEnvironmentHeartbeat(ctx, input.ID); err != nil {
		return nil, huma.Error500InternalServerError("Failed to update heartbeat")
	}

	return &handlerutil.Out[base.MessageResponse]{
		Body: base.ApiResponse[base.MessageResponse]{
			Success: true,
			Data: base.MessageResponse{
				Message: "Heartbeat updated successfully",
			},
		},
	}, nil
}

// PairAgent generates or rotates the local agent pairing token.
func (h *EnvironmentHandler) PairAgent(ctx context.Context, input *PairAgentInput) (*handlerutil.Out[environment.AgentPairResponse], error) {
	if input.ID != localDockerEnvironmentID {
		return nil, huma.Error404NotFound("Not found")
	}

	shouldRotate := input.Body != nil && input.Body.Rotate != nil && *input.Body.Rotate
	if h.cfg.AgentToken == "" || shouldRotate {
		h.cfg.AgentToken = kit.RandomString(48)
	}

	if err := h.settingsService.SetStringSetting(ctx, "agentToken", h.cfg.AgentToken); err != nil {
		return nil, huma.Error500InternalServerError("Failed to persist agent token")
	}

	return &handlerutil.Out[environment.AgentPairResponse]{
		Body: base.ApiResponse[environment.AgentPairResponse]{
			Success: true,
			Data: environment.AgentPairResponse{
				Token: h.cfg.AgentToken,
			},
		},
	}, nil
}

// SyncEnvironment syncs manager-owned resources to an environment.
func (h *EnvironmentHandler) SyncEnvironment(ctx context.Context, input *SyncEnvironmentInput) (*handlerutil.Out[base.MessageResponse], error) {
	user, err := handlerutil.RequireUser(ctx)
	if err != nil {
		return nil, err
	}

	runtimeCtx := utils.ActivityRuntimeContext(ctx, h.appCtx)
	activityID, err := h.environmentService.SyncResourcesToEnvironment(runtimeCtx, input.ID, user, h.activityService)
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}

	return &handlerutil.Out[base.MessageResponse]{
		Body: base.ApiResponse[base.MessageResponse]{
			Success: true,
			Data: base.MessageResponse{
				Message:    "Environment synced successfully",
				ActivityID: mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer(),
			},
		},
	}, nil
}

func (h *EnvironmentHandler) buildUpdateMapInternal(req *environment.Update, isLocalEnv bool) map[string]any {
	updates := map[string]any{}

	if !isLocalEnv {
		if req.ApiUrl != nil {
			updates["api_url"] = *req.ApiUrl
		}
		if req.Enabled != nil {
			updates["enabled"] = *req.Enabled
		}
	}

	if req.Name != nil {
		updates["name"] = *req.Name
	}

	return updates
}

func (h *EnvironmentHandler) handleEnvironmentPairingInternal(ctx context.Context, environmentID string, req *environment.Update, updates map[string]any, isLocalEnv bool) {
	_ = ctx
	_ = environmentID
	if isLocalEnv {
		return
	}

	if req.AccessToken != nil {
		updates["access_token"] = *req.AccessToken
	}
}

func (h *EnvironmentHandler) triggerPostUpdateTasksInternal(ctx context.Context, environmentID string, updated *Environment, req *environment.Update) {
	if updated.Enabled {
		detachedCtx := context.WithoutCancel(ctx)
		h.background.Add(1)
		go func(syncCtx context.Context, envID, envName string) {
			defer h.background.Done()
			status, err := h.environmentService.TestConnection(syncCtx, envID, nil)
			if err != nil {
				slog.WarnContext(syncCtx, "Failed to test connection after environment update",
					"environmentId", envID, "environmentName", envName, "status", status, "error", err)
			}
		}(detachedCtx, environmentID, updated.Name)
	}

	if updated.AccessToken != nil && *updated.AccessToken != "" && ((req.AccessToken != nil && *req.AccessToken != "") || req.Name != nil) {
		h.triggerEnvironmentResourceSyncInternal(ctx, environmentID, updated.Name, "environment update")
	}
}

func (h *EnvironmentHandler) triggerEnvironmentResourceSyncInternal(ctx context.Context, environmentID, environmentName, reason string) {
	h.environmentService.ForgetSyncState(environmentID)
	detachedCtx := context.WithoutCancel(ctx)

	h.background.Add(1)
	go func(syncCtx context.Context, envID, envName, syncReason string) {
		defer h.background.Done()
		syncCtx, cancel := context.WithTimeout(syncCtx, edge.DefaultProxyTimeout)
		defer cancel()
		if err := h.environmentService.SyncRegistriesToEnvironment(syncCtx, envID); err != nil {
			slog.WarnContext(syncCtx, "Failed to sync registries to environment",
				"environmentId", envID,
				"environmentName", envName,
				"reason", syncReason,
				"error", err.Error())
		}
	}(detachedCtx, environmentID, environmentName, reason)

}

// PairEnvironment handles agent pairing callback with API key.
func (h *EnvironmentHandler) PairEnvironment(ctx context.Context, input *PairEnvironmentInput) (*handlerutil.Out[base.MessageResponse], error) {
	if input.XAPIKey == "" {
		return nil, huma.Error400BadRequest("X-API-Key header is required")
	}

	envID, err := h.apiKeyService.GetEnvironmentByApiKey(ctx, input.XAPIKey)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to validate API key for pairing", "error", err.Error())
		return nil, huma.Error401Unauthorized("Invalid API key")
	}

	if envID == nil {
		return nil, huma.Error400BadRequest("API key is not linked to an environment")
	}

	env, err := h.environmentService.GetEnvironmentByID(ctx, *envID)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get environment", "environmentId", *envID, "error", err.Error())
		return nil, huma.Error404NotFound("Environment not found")
	}

	if env.Status != string(EnvironmentStatusPending) {
		return nil, huma.Error400BadRequest("Environment is not in pending status")
	}

	updates := map[string]any{
		"status": string(EnvironmentStatusPending),
	}
	_, err = h.environmentService.UpdateEnvironment(ctx, *envID, updates, nil, nil)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to update environment status", "environmentId", *envID, "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to complete pairing")
	}

	slog.InfoContext(ctx, "Environment pairing completed", "environmentId", *envID, "environmentName", env.Name)
	h.triggerEnvironmentResourceSyncInternal(ctx, *envID, env.Name, "environment pairing")

	return &handlerutil.Out[base.MessageResponse]{
		Body: base.ApiResponse[base.MessageResponse]{
			Success: true,
			Data: base.MessageResponse{
				Message: "Environment pairing completed successfully",
			},
		},
	}, nil
}
