// Source: docklet-next/issues/466
// Revision: b4f894ff7f35186d4c39be1fc9fccd01b4b5934a
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original protected workspace refusal through Direct and Edge.

package project

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	dto "github.com/florianfinn/docklet-next/types/project"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceProtectedEnvRefusalDirectEdge(t *testing.T) {
	for _, transport := range []string{"Direct", "Edge"} {
		t.Run(transport, func(t *testing.T) {
			f := newLifecycleFixture(t)
			require.NoError(t, os.Remove(filepath.Join(f.project.Path, ".env")))
			workspace, err := f.service.GetProjectWorkspace(t.Context(), f.project.ID)
			require.NoError(t, err)
			agent := echo.New()
			api := humaecho.NewWithGroup(agent, agent.Group("/api"), huma.DefaultConfig("workspace", "test"))
			api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
				request := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
				request = context.WithValue(request, user.CurrentUserContextKey{}, &user.SystemUser)
				next(huma.WithContext(ctx, request))
			})
			registerProjectWorkspaceRoutesInternal(api, &ProjectHandler{projectService: f.service, activityService: f.activity, appCtx: t.Context()})
			server := httptest.NewServer(agent)
			t.Cleanup(server.Close)
			registry := edge.NewTunnelRegistry()
			endpoint := server.URL
			token := rand.Text()
			if transport == "Edge" {
				registry.Register("remote", systemProtectionTunnel(t, agent, token))
				endpoint = "edge://example"
			}
			hub := echo.New()
			hub.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) { return endpoint, &token, true, nil }, func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) {
				return authz.SudoPermissionSet(), &user.SystemUser, true
			}, nil, registry, nil, middleware.EnvironmentProxySecurity{HTTPClient: server.Client()}))
			hub.PUT("/api/environments/:id/*", func(c *echo.Context) error { return c.NoContent(500) })
			manifest := dto.WorkspaceUpdateManifest{FileTreeRevision: workspace.FileTreeRevision, FileChanges: []dto.WorkspaceFileChange{{Operation: dto.FileOpCreateFile, RelativePath: ".env", UploadIndex: new(0)}}}
			encoded, err := json.Marshal(manifest)
			require.NoError(t, err)
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			require.NoError(t, writer.WriteField("manifest", string(encoded)))
			part, err := writer.CreateFormFile("files", "upload.txt")
			require.NoError(t, err)
			_, err = part.Write([]byte("SYNTHETIC_VALUE=example\n"))
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			request := httptest.NewRequest(http.MethodPut, "/api/environments/remote/projects/"+f.project.ID+"/workspace", &body)
			request.Header.Set("Content-Type", writer.FormDataContentType())
			response := httptest.NewRecorder()
			hub.ServeHTTP(response, request)
			require.Equal(t, 422, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"code":"project.workspace-protected-path"`)
			require.NotContains(t, response.Body.String(), "agent.invalid-token")
			require.NotContains(t, response.Body.String(), f.project.Path)
			require.NoFileExists(t, filepath.Join(f.project.Path, ".env"))
		})
	}
}
