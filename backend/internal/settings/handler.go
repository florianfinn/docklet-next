// Source: getarcaneapp/arcane/backend/internal/settings/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed Swarm; functional cut: removed OIDC and passkeys. #69 first-run setup and hub settings persistence; notification and Auto Heal contracts and delivery for #296; M14 container and host operation contracts; typed UI-configuration refusal producer.

package settings

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	autohealDTO "github.com/florianfinn/docklet-next/types/autoheal"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/category"
	environmentDTO "github.com/florianfinn/docklet-next/types/environment"
	searchtypes "github.com/florianfinn/docklet-next/types/search"
	"github.com/florianfinn/docklet-next/types/settings"
	"go.getarcane.app/kit/pkg"
	"go.getarcane.app/kit/pkg/mapping"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/search"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/workspace"
)

const (
	projectWorkspaceMaxFileSizeSettingKey = "projectWorkspaceMaxFileSizeMb"
	volumeWorkspaceMaxFileSizeSettingKey  = "volumeWorkspaceMaxFileSizeMb"
)

// SettingsHandler provides Huma-based settings management endpoints.
type SettingsHandler struct {
	settingsService       *SettingsService
	settingsSearchService *SettingsSearchService
	proxyRemoteJSON       handlerutil.RemoteJSONProxy
	cfg                   *config.Config
}

type GetSettingsInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
}

type GetSettingsOutput struct {
	Body []settings.PublicSetting `doc:"Settings including the read-only effectiveTimeZone from TZ"`
}

type GetPublicSettingsInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
}

type GetPublicSettingsOutput struct {
	Body []settings.PublicSetting
}

type UpdateSettingsInput struct {
	EnvironmentID string          `path:"id" doc:"Environment ID"`
	Body          settings.Update `doc:"Settings update data"`
}

type SearchSettingsInput struct {
	Body searchtypes.Request `doc:"Search query"`
}

type SearchSettingsOutput struct {
	Body searchtypes.Response
}

type GetCategoriesOutput struct {
	Body []category.Category
}

// validateProjectsDirectoryValueInternal validates a projects directory value allowing:
// - Unix absolute paths (/...)
// - Windows drive paths (C:/..., C:\...)
// - Mapping format "container:host" where container is absolute Unix or Windows path
func validateProjectsDirectoryValueInternal(path string) error {
	switch {
	case projects.IsWindowsDrivePath(path):
		return nil
	case strings.Contains(path, ":"):
		parts := strings.SplitN(path, ":", 2)
		if len(parts) != 2 {
			return errors.New("projectsDirectory must be an absolute path or valid mapping format")
		}
		container := parts[0]
		if !strings.HasPrefix(container, "/") && !projects.IsWindowsDrivePath(container) {
			return errors.New("projectsDirectory mapping format: container path must be absolute")
		}
		return nil
	default:
		if !strings.HasPrefix(path, "/") {
			return errors.New("projectsDirectory must be an absolute path starting with '/'")
		}
		return nil
	}
}

// validateAbsoluteDirectoryPathInternal validates a plain absolute directory path allowing:
// - Unix absolute paths (/...)
// - Windows drive paths (C:/..., C:\...)
func validateAbsoluteDirectoryPathInternal(path string) error {
	switch {
	case projects.IsWindowsDrivePath(path):
		return nil
	case strings.HasPrefix(path, "/"):
		return nil
	default:
		return errors.New("must be an absolute path")
	}
}

func filterSettingsCategoriesInternal(ps *authz.PermissionSet, categories []category.Category) []category.Category {
	if ps == nil {
		return []category.Category{}
	}
	filtered := make([]category.Category, 0, len(categories))
	for _, cat := range categories {
		if canAccessSettingsCategoryAtAnyScopeInternal(ps, cat.ID) {
			filtered = append(filtered, cat)
		}
	}
	return filtered
}

func canAccessSettingsCategoryAtAnyScopeInternal(ps *authz.PermissionSet, categoryID string) bool {
	if ps == nil {
		return false
	}
	if authz.CanAccessSettingsCategory(ps, categoryID, "") {
		return true
	}
	for envID := range ps.PerEnv {
		if authz.CanAccessSettingsCategory(ps, categoryID, envID) {
			return true
		}
	}
	return false
}

func (h *SettingsHandler) appendRuntimeSettingsInternal(settingsDto []settings.PublicSetting, includeAuthenticatedOnly bool) []settings.PublicSetting {
	if h.cfg != nil && (h.cfg.AgentMode || h.cfg.EdgeAgent) {
		settingsDto = withoutStackStopMode(settingsDto)
	}
	if !includeAuthenticatedOnly {
		return settingsDto
	}

	cfg := h.cfg
	if cfg == nil {
		cfg = config.Load()
	}
	settingsDto = append(settingsDto, settings.PublicSetting{Key: "effectiveTimeZone", Value: cfg.GetLocation().String(), Type: "string"})

	uiConfigDisabled := false
	if h.cfg != nil {
		uiConfigDisabled = h.cfg.UIConfigurationDisabled
	}
	settingsDto = append(settingsDto, settings.PublicSetting{
		Key:   "uiConfigDisabled",
		Value: strconv.FormatBool(uiConfigDisabled),
		Type:  "boolean",
	})

	projectWorkspaceMaxFileSizeMB := 10
	volumeWorkspaceMaxFileSizeMB := 10
	if h.cfg != nil {
		projectWorkspaceMaxFileSizeMB = workspace.EffectiveMaxFileSizeMB(h.cfg.ProjectWorkspaceMaxFileSizeMB)
		volumeWorkspaceMaxFileSizeMB = workspace.EffectiveMaxFileSizeMB(h.cfg.VolumeWorkspaceMaxFileSizeMB)
	}
	settingsDto = append(settingsDto,
		settings.PublicSetting{Key: projectWorkspaceMaxFileSizeSettingKey, Value: strconv.Itoa(projectWorkspaceMaxFileSizeMB), Type: "number"},
		settings.PublicSetting{Key: volumeWorkspaceMaxFileSizeSettingKey, Value: strconv.Itoa(volumeWorkspaceMaxFileSizeMB), Type: "number"},
	)

	backupVolumeName := "arcane-backups"
	if h.cfg != nil && strings.TrimSpace(h.cfg.BackupVolumeName) != "" {
		backupVolumeName = h.cfg.BackupVolumeName
	}
	settingsDto = append(settingsDto, settings.PublicSetting{
		Key:   "backupVolumeName",
		Value: backupVolumeName,
		Type:  "string",
	})

	var edgeCfg *edge.Config
	if h.cfg != nil {
		edgeCfg = &edge.Config{
			EdgeMTLSMode:      h.cfg.EdgeMTLSMode,
			EdgeMTLSCAFile:    h.cfg.EdgeMTLSCAFile,
			EdgeMTLSAssetsDir: h.cfg.EdgeMTLSAssetsDir,
		}
	}
	_, edgeMTLSCAErr := edge.AvailableManagerMTLSCAPath(edgeCfg)
	settingsDto = append(settingsDto, settings.PublicSetting{
		Key:   "edgeMTLSManagerCAAvailable",
		Value: strconv.FormatBool(edgeMTLSCAErr == nil),
		Type:  "boolean",
	})

	if h.settingsService != nil {
		cfg := h.settingsService.GetSettingsConfig()
		depotConfigured := strings.TrimSpace(cfg.DepotProjectId.Value) != "" && strings.TrimSpace(cfg.DepotToken.Value) != ""
		settingsDto = append(settingsDto, settings.PublicSetting{
			Key:   "depotConfigured",
			Value: strconv.FormatBool(depotConfigured),
			Type:  "boolean",
		})
	}

	return settingsDto
}

// GetPublicSettings returns public settings for an environment.
func (h *SettingsHandler) GetPublicSettings(ctx context.Context, input *GetPublicSettingsInput) (*GetPublicSettingsOutput, error) {
	if input.EnvironmentID != "0" {
		settingsDto, err := h.proxyRemoteJSON.JSON[[]settings.PublicSetting](ctx, input.EnvironmentID, http.MethodGet, "/api/environments/0/settings/public", nil)
		if err != nil {
			return nil, err
		}
		return &GetPublicSettingsOutput{Body: *settingsDto}, nil
	}

	settingsList := h.settingsService.ListSettings(SettingVisibilityPublic)

	settingsDto, err := mapping.MapSlice[SettingVariable, settings.PublicSetting](settingsList)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to map settings")
	}

	return &GetPublicSettingsOutput{Body: h.appendRuntimeSettingsInternal(settingsDto, false)}, nil
}

// GetSettings returns all settings for an environment.
func (h *SettingsHandler) GetSettings(ctx context.Context, input *GetSettingsInput) (*GetSettingsOutput, error) {
	ps, _ := middleware.PermissionsFromContext(ctx)
	isAdmin := ps.IsGlobalAdmin()
	visibility := kit.Ternary(isAdmin, SettingVisibilityAll, SettingVisibilityNonAdmin)

	if input.EnvironmentID != "0" {
		settingsDto, err := h.proxyRemoteJSON.JSON[[]settings.PublicSetting](ctx, input.EnvironmentID, http.MethodGet, "/api/environments/0/settings", nil)
		if err != nil {
			return nil, err
		}
		if !isAdmin {
			allowedKeys := make(map[string]struct{})
			for _, setting := range h.settingsService.ListSettings(visibility) {
				allowedKeys[setting.Key] = struct{}{}
			}
			allowedKeys[projectWorkspaceMaxFileSizeSettingKey] = struct{}{}
			allowedKeys[volumeWorkspaceMaxFileSizeSettingKey] = struct{}{}
			filtered := make([]settings.PublicSetting, 0, len(*settingsDto))
			for _, setting := range *settingsDto {
				if _, ok := allowedKeys[setting.Key]; ok {
					filtered = append(filtered, setting)
				}
			}
			settingsDto = &filtered
		}
		return &GetSettingsOutput{Body: withoutStackStopMode(*settingsDto)}, nil
	}

	settingsList := h.settingsService.ListSettings(visibility)

	settingsDto, err := mapping.MapSlice[SettingVariable, settings.PublicSetting](settingsList)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to map settings")
	}

	return &GetSettingsOutput{Body: h.appendRuntimeSettingsInternal(settingsDto, true)}, nil
}

// UpdateSettings updates settings for an environment.
func (h *SettingsHandler) UpdateSettings(ctx context.Context, input *UpdateSettingsInput) (*handlerutil.Out[[]settings.SettingDto], error) {
	if input.EnvironmentID == "0" && h.cfg != nil && h.cfg.UIConfigurationDisabled {
		return nil, &environmentDTO.HostProblem{Code: "settings.ui-configuration-disabled", Status: 403}
	}
	if input.Body.StackStopMode != nil && (input.EnvironmentID != "0" || (h.cfg != nil && (h.cfg.AgentMode || h.cfg.EdgeAgent))) {
		return nil, huma.Error400BadRequest("stackStopMode is a hub-only setting")
	}
	if err := h.validateSettingsUpdateInput(ctx, input.Body); err != nil {
		return nil, err
	}

	if input.EnvironmentID != "0" {
		return h.updateSettingsForRemoteEnvironment(ctx, input)
	}

	return h.updateSettingsForLocalEnvironment(ctx, input.Body)
}

func (h *SettingsHandler) validateSettingsUpdateInput(ctx context.Context, input settings.Update) error {
	if AutoHealUpdateManaged(input) {
		return &autohealDTO.Problem{Code: "settings.key-managed", Status: 400}
	}
	if input.SetupCompleted != nil && *input.SetupCompleted == "true" && (h.cfg == nil || h.cfg.SignInMode() == "local") {
		userID, ok := middleware.GetUserIDFromContext(ctx)
		if !ok {
			return huma.Error401Unauthorized("Sign-in required")
		}
		var requiresPasswordChange bool
		if err := h.settingsService.db.WithContext(ctx).Table("users").Select("requires_password_change").Where("id = ?", userID).Scan(&requiresPasswordChange).Error; err != nil {
			return huma.Error500InternalServerError("Failed to check administrator password")
		}
		if requiresPasswordChange {
			return huma.Error409Conflict("Change the initial administrator password before completing setup")
		}
	}
	// Validate projects directory if provided and changed from current value.
	// Skip validation when the value matches the current (possibly env-overridden) setting
	// so that saving unrelated settings doesn't fail due to env-provided directory formats.
	if input.ProjectsDirectory != nil && *input.ProjectsDirectory != "" {
		currentDir := h.settingsService.GetSettingsConfig().ProjectsDirectory.Value
		if *input.ProjectsDirectory != currentDir {
			if err := validateProjectsDirectoryValueInternal(*input.ProjectsDirectory); err != nil {
				return huma.Error400BadRequest(err.Error())
			}
		}
	}

	if input.AvatarMaxUploadSizeMb != nil && strings.TrimSpace(*input.AvatarMaxUploadSizeMb) != "" {
		value, err := strconv.Atoi(strings.TrimSpace(*input.AvatarMaxUploadSizeMb))
		if err != nil || value < 1 || value > 50 {
			return huma.Error400BadRequest("avatarMaxUploadSizeMb must be between 1 and 50")
		}
	}

	return nil
}

func (h *SettingsHandler) updateSettingsForRemoteEnvironment(ctx context.Context, input *UpdateSettingsInput) (*handlerutil.Out[[]settings.SettingDto], error) {
	// Check if trying to update auth settings on non-local environment.
	if hasAuthSettingsUpdateInternal(input.Body) {
		return nil, huma.Error403Forbidden("Authentication settings can only be updated from the main environment")
	}

	apiResp, err := h.proxyRemoteJSON.JSON[base.ApiResponse[[]settings.SettingDto]](ctx, input.EnvironmentID, http.MethodPut, "/api/environments/0/settings", input.Body)
	if err != nil {
		return nil, err
	}

	return &handlerutil.Out[[]settings.SettingDto]{Body: *apiResp}, nil
}

func (h *SettingsHandler) updateSettingsForLocalEnvironment(ctx context.Context, input settings.Update) (*handlerutil.Out[[]settings.SettingDto], error) {
	if input.ProjectsDirectory != nil && *input.ProjectsDirectory != "" {
		currentDir := h.settingsService.GetSettingsConfig().ProjectsDirectory.Value
		if *input.ProjectsDirectory != currentDir {
			resolved, err := projects.GetProjectsDirectory(ctx, strings.TrimSpace(*input.ProjectsDirectory))
			if err != nil {
				return nil, huma.Error400BadRequest(fmt.Sprintf("cannot use projects directory %q: %v", *input.ProjectsDirectory, err))
			}
			// os.* rather than acfs: this validates a user-typed directory before
			// it becomes a confinement root, so no root exists for it yet.
			f, err := os.Open(resolved)
			if err != nil {
				return nil, huma.Error400BadRequest(fmt.Sprintf("cannot read projects directory %q: %v", resolved, err))
			}
			if closeErr := f.Close(); closeErr != nil {
				return nil, huma.Error400BadRequest(fmt.Sprintf("cannot read projects directory %q: %v", resolved, closeErr))
			}
		}
	}

	updatedSettings, err := h.settingsService.UpdateSettings(ctx, input)
	if err != nil {
		apiErr := common.ToAPIError(err)
		if apiErr.HTTPStatus() == http.StatusInternalServerError {
			return nil, huma.Error500InternalServerError("Failed to update settings")
		}
		return nil, huma.NewError(apiErr.HTTPStatus(), apiErr.Message)
	}

	settingDtos := make([]settings.SettingDto, 0, len(updatedSettings))
	for _, setting := range updatedSettings {
		settingDtos = append(settingDtos, settings.SettingDto{
			Key:   setting.Key,
			Type:  "string",
			Value: setting.Value,
		})
	}

	return &handlerutil.Out[[]settings.SettingDto]{
		Body: base.ApiResponse[[]settings.SettingDto]{
			Success: true,
			Data:    settingDtos,
		},
	}, nil
}

func hasAuthSettingsUpdateInternal(req settings.Update) bool {
	return req.AuthLocalEnabled != nil || req.AuthSessionTimeout != nil || req.AuthPasswordPolicy != nil
}

// Search searches settings by query.
func (h *SettingsHandler) Search(ctx context.Context, input *SearchSettingsInput) (*SearchSettingsOutput, error) {
	if strings.TrimSpace(input.Body.Query) == "" {
		return nil, huma.Error400BadRequest("Query parameter is required")
	}

	ps, _ := middleware.PermissionsFromContext(ctx)
	results := search.Search(h.settingsSearchService.GetSettingsCategories(), input.Body.Query, searchtypes.SettingsProfile)
	results.Results = filterSettingsCategoriesInternal(ps, results.Results)
	results.Count = len(results.Results)
	return &SearchSettingsOutput{Body: results}, nil
}

// GetCategories returns all available settings categories.
func (h *SettingsHandler) GetCategories(ctx context.Context, input *struct{}) (*GetCategoriesOutput, error) {
	ps, _ := middleware.PermissionsFromContext(ctx)
	categories := filterSettingsCategoriesInternal(ps, h.settingsSearchService.GetSettingsCategories())
	return &GetCategoriesOutput{Body: categories}, nil
}

func withoutStackStopMode(values []settings.PublicSetting) []settings.PublicSetting {
	out := make([]settings.PublicSetting, 0, len(values))
	for _, setting := range values {
		if setting.Key != "stackStopMode" {
			out = append(out, setting)
		}
	}
	return out
}
