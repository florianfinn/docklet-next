// Source: getarcaneapp/arcane/backend/internal/playwright/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed federated credentials.

//go:build playwright

package playwright

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

type playwrightHandlerInternal struct {
	service *PlaywrightService
}

type createTestAPIKeysRequestInternal struct {
	Count int `json:"count"`
}

// SetupRoutes registers endpoints used by the Playwright end-to-end suite.
func SetupRoutes(api *echo.Group, service *PlaywrightService) {
	group := api.Group("/playwright")
	handler := &playwrightHandlerInternal{service: service}

	group.POST("/create-test-gitops-project", handler.createTestGitOpsProjectInternal)
	group.POST("/create-test-api-keys", handler.createTestAPIKeysInternal)
	group.POST("/delete-test-api-keys", handler.deleteTestAPIKeysInternal)
}

func (h *playwrightHandlerInternal) createTestAPIKeysInternal(c *echo.Context) error {
	var req createTestAPIKeysRequestInternal
	if err := c.Bind(&req); err != nil || req.Count <= 0 {
		req.Count = 2
	}

	apiKeys, err := h.service.CreateTestApiKeys(c.Request().Context(), req.Count)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, map[string]any{"apiKeys": apiKeys})
}

func (h *playwrightHandlerInternal) deleteTestAPIKeysInternal(c *echo.Context) error {
	if err := h.service.DeleteAllTestApiKeys(c.Request().Context()); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *playwrightHandlerInternal) createTestGitOpsProjectInternal(c *echo.Context) error {
	if err := h.service.CreateTestGitOpsProject(c.Request().Context()); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}
