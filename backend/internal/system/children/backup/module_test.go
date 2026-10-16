// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original registered-operation permission and stub-response contract test for issue #32.

package backup

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
)

func TestHubBackupRegisteredPermissionsAndStubs(t *testing.T) {
	rows := []struct {
		method, path, permission, body, contentType string
		stub                                        bool
	}{
		{http.MethodGet, "/backups", authz.PermSystemBackupsRead, "", "", false},
		{http.MethodGet, "/backups/history", authz.PermSystemBackupsRead, "", "", false},
		{http.MethodGet, "/backups/policies", authz.PermSystemBackupsRead, "", "", false},
		{http.MethodPut, "/backups/policies", authz.PermSystemBackupsManage, `{"policies":[]}`, "application/json", true},
		{http.MethodGet, "/backups/storage", authz.PermSystemBackupsRead, "", "", true},
		{http.MethodGet, "/backups/maintenance", authz.PermSystemBackupsRead, "", "", true},
		{http.MethodPost, "/backups", authz.PermSystemBackupsManage, `{}`, "application/json", true},
		{http.MethodPost, "/backups/upload", authz.PermSystemBackupsRestore, "archive", "application/octet-stream", true},
		{http.MethodGet, "/backups/{id}/download", authz.PermSystemBackupsRead, "", "", true},
		{http.MethodPost, "/backups/{id}/restore", authz.PermSystemBackupsRestore, `{"confirmBackupId":"backup1"}`, "application/json", true},
		{http.MethodDelete, "/backups/{id}", authz.PermSystemBackupsManage, "", "", true},
	}
	router := echo.New()
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("test", "1.0.0"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		permissions := authz.NewPermissionSet()
		if ctx.Header("X-Test-Admin") == "true" {
			permissions = authz.SudoPermissionSet()
		}
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, permissions)))
	})
	RegisterRoutes(api, NewHandler(nil, nil, t.Context()))
	registered := 0
	for _, path := range api.OpenAPI().Paths {
		for _, op := range []*huma.Operation{path.Get, path.Post, path.Put, path.Delete, path.Patch} {
			if op != nil && slices.Contains(op.Tags, "Hub Backups") {
				registered++
			}
		}
	}
	require.Equal(t, len(rows), registered)

	for _, row := range rows {
		t.Run(row.method+" "+row.path, func(t *testing.T) {
			path := api.OpenAPI().Paths[row.path]
			require.NotNil(t, path)
			var op *huma.Operation
			switch row.method {
			case http.MethodGet:
				op = path.Get
			case http.MethodPost:
				op = path.Post
			case http.MethodPut:
				op = path.Put
			case http.MethodDelete:
				op = path.Delete
			}
			require.NotNil(t, op)
			require.Equal(t, row.permission, op.Metadata[authz.MetaRequiredPermission])
			if row.path == "/backups/upload" {
				require.Equal(t, backuptypes.MaxSystemArchiveBytes+1, op.MaxBodyBytes)
			}
			requestPath := strings.ReplaceAll(row.path, "{id}", "backup1")
			request := func(admin bool) *httptest.ResponseRecorder {
				req := httptest.NewRequest(row.method, "/api"+requestPath, bytes.NewBufferString(row.body))
				if row.contentType != "" {
					req.Header.Set("Content-Type", row.contentType)
				}
				if admin {
					req.Header.Set("X-Test-Admin", "true")
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				return rec
			}
			require.Equal(t, http.StatusForbidden, request(false).Code)
			if !row.stub {
				return
			}
			rec := request(true)
			require.Equal(t, http.StatusNotImplemented, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "backup.not_implemented")
		})
	}
}

func TestUploadRejectsUnauthorizedTypeAndDeclaredOversize(t *testing.T) {
	router := echo.New()
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("test", "1.0.0"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		permissions := authz.NewPermissionSet()
		if ctx.Header("X-Test-Admin") == "true" {
			permissions = authz.SudoPermissionSet()
		}
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, permissions)))
	})
	RegisterRoutes(api, NewHandler(&Service{}, nil, t.Context()))
	request := func(admin bool, contentType string, length int64) int {
		req := httptest.NewRequest(http.MethodPost, "/api/backups/upload", bytes.NewBufferString("PGDMP"))
		req.Header.Set("Content-Type", contentType)
		req.ContentLength = length
		if admin {
			req.Header.Set("X-Test-Admin", "true")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}
	require.Equal(t, http.StatusForbidden, request(false, "application/octet-stream", 5))
	require.Equal(t, http.StatusUnsupportedMediaType, request(true, "application/json", 5))
	require.Equal(t, http.StatusRequestEntityTooLarge, request(true, "application/octet-stream", backuptypes.MaxSystemArchiveBytes+1))
}
