// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original local upload-restore dual-permission regression.

package restore

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/upload"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"uuid"
)

func TestUploadRestoreRequiresUploadAndBackupBeforeSessionAccess(t *testing.T) {
	for _, tc := range []struct {
		name        string
		permissions []string
		want        int
	}{{"neither", nil, 403}, {"upload-only", []string{authz.PermVolumesUpload}, 403}, {"backup-only", []string{authz.PermVolumesBackup}, 403}, {"both", []string{authz.PermVolumesUpload, authz.PermVolumesBackup}, 404}} {
		t.Run(tc.name, func(t *testing.T) {
			router := echo.New()
			router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c *echo.Context) error {
					ps := authz.NewPermissionSet()
					ps.AddEnv("0", tc.permissions...)
					ctx := context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, ps)
					ctx = context.WithValue(ctx, user.CurrentUserContextKey{}, &user.SystemUser)
					c.SetRequest(c.Request().WithContext(ctx))
					return next(c)
				}
			})
			api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("backup", "test"))
			RegisterRoutes(api, NewHandler(nil, nil, upload.NewUploadService(nil), t.Context()))
			req := httptest.NewRequest(http.MethodPost, "/api/environments/0/volumes/data/backups/upload", strings.NewReader(`{"uploadId":"`+uuid.New().String()+`"}`))
			req.Header.Set("Content-Type", "application/json")
			out := httptest.NewRecorder()
			router.ServeHTTP(out, req)
			require.Equal(t, tc.want, out.Code, out.Body.String())
			if tc.want == 403 {
				body := &permissionProbeReader{}
				req = httptest.NewRequest(http.MethodPost, "/api/environments/0/volumes/data/backups/upload", body)
				req.Header.Set("Content-Type", "multipart/form-data; boundary=example")
				out = httptest.NewRecorder()
				router.ServeHTTP(out, req)
				require.Equal(t, 403, out.Code, out.Body.String())
				require.Zero(t, body.reads, "permissions must precede legacy multipart staging")
			}
		})
	}
}

type permissionProbeReader struct{ reads int }

func (r *permissionProbeReader) Read([]byte) (int, error) { r.reads++; return 0, io.EOF }
