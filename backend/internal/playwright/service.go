// Source: getarcaneapp/arcane/backend/internal/playwright/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed GitOps and saved Git repositories.

//go:build playwright

package playwright

import (
	"context"
	"fmt"
	"log/slog"

	apikeytypes "github.com/florianfinn/docklet-next/types/apikey"

	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
)

type PlaywrightService struct {
	apiKeyService *apikey.ApiKeyService
	userService   *user.UserService
}

func NewPlaywrightService(
	apiKeyService *apikey.ApiKeyService,
	userService *user.UserService,
) *PlaywrightService {
	return &PlaywrightService{
		apiKeyService: apiKeyService,
		userService:   userService,
	}
}

func (ps *PlaywrightService) CreateTestApiKeys(ctx context.Context, count int) ([]*apikeytypes.ApiKeyCreatedDto, error) {
	slog.InfoContext(ctx, "Playwright: Creating test API keys", "count", count)

	// Get the arcane user to associate the API keys with
	localUser, err := ps.userService.GetUserByUsername(ctx, "arcane")
	if err != nil {
		return nil, fmt.Errorf("failed to get arcane user: %w", err)
	}

	// Grant every recognized permission globally so the test key behaves like
	// the legacy "admin-everywhere" credential the e2e suite expects. There is
	// no request context here, so grant validation runs against a sudo set.
	allPerms := authz.AllPermissions()
	grants := make([]apikeytypes.PermissionGrant, len(allPerms))
	for i, p := range allPerms {
		grants[i] = apikeytypes.PermissionGrant{Permission: p}
	}

	var createdKeys []*apikeytypes.ApiKeyCreatedDto
	for i := range count {
		description := fmt.Sprintf("Test API key %d for Playwright tests", i+1)
		req := apikeytypes.CreateApiKey{
			Name:        fmt.Sprintf("test-api-key-%d", i+1),
			Description: &description,
			Permissions: grants,
		}

		apiKey, createApiKeyErr := ps.apiKeyService.CreateApiKey(ctx, localUser.ID, authz.SudoPermissionSet(), req)
		if createApiKeyErr != nil {
			return nil, fmt.Errorf("failed to create test API key %d: %w", i+1, createApiKeyErr)
		}

		createdKeys = append(createdKeys, apiKey)
	}

	slog.InfoContext(ctx, "Playwright: Test API keys created successfully", "count", len(createdKeys))
	return createdKeys, nil
}

func (ps *PlaywrightService) DeleteAllTestApiKeys(ctx context.Context) error {
	slog.InfoContext(ctx, "Playwright: Deleting all test API keys")

	// Get all API keys with test prefix
	params := pagination.QueryParams{
		Search: "test-api-key",
		Start:  0,
		Limit:  1000,
	}

	apiKeys, _, err := ps.apiKeyService.ListApiKeys(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to list API keys: %w", err)
	}

	for _, apiKey := range apiKeys {
		if deleteApiKeyErr := ps.apiKeyService.DeleteApiKey(ctx, apiKey.ID); deleteApiKeyErr != nil {
			slog.WarnContext(ctx, "Failed to delete test API key", "id", apiKey.ID, "error", deleteApiKeyErr)
		}
	}

	slog.InfoContext(ctx, "Playwright: Test API keys deleted", "count", len(apiKeys))
	return nil
}
