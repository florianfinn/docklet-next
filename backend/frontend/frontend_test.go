// Source: getarcaneapp/arcane/backend/frontend/frontend_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: ; functional cut: removed Svelte-specific metadata or asset paths.

//go:build !exclude_frontend

package frontend

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

// TestRegisterFrontend_ServesSPA checks client routes, missing assets and API routes.
func TestRegisterFrontend_ServesSPA(t *testing.T) {
	e := echo.New()
	e.GET("/api/health", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	require.NoError(t, RegisterFrontend(e))

	for _, path := range []string{"/", "/containers/some-id"} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, rec.Code, "path %s", path)
		require.Contains(t, rec.Body.String(), "<!doctype html", "path %s should serve index.html", path)
	}

	missingChunk := httptest.NewRecorder()
	e.ServeHTTP(missingChunk, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	require.Equal(t, http.StatusNotFound, missingChunk.Code)
	require.NotContains(t, missingChunk.Body.String(), "<!doctype html")

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, strings.HasPrefix(rec.Body.String(), "ok"))
}
