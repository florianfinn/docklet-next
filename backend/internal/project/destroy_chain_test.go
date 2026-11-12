// Source: docklet-next/issues/434
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original hub-local, Direct and Edge stack destroy chain through the real handler and tunnel.

package project

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

type destroyRoundTrip func(*http.Request) (*http.Response, error)

func (f destroyRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type destroyCleanup struct{ env, stack string }

// destroyChain serves a hub request on one host kind. Hub-local calls the agent
// router directly; Direct and Edge pass the real environment middleware, and Edge
// also the real WebSocket tunnel server and client.
func destroyChain(t *testing.T, mode string, agent http.Handler) (string, http.Handler, *[]destroyCleanup) {
	t.Helper()
	cleanups := &[]destroyCleanup{}
	if mode == "local" {
		return "0", agent, cleanups
	}
	envID, apiURL := "direct-host", "https://agent.example.com"
	registry := edge.NewTunnelRegistry()
	options := middleware.EnvironmentProxySecurity{PolicyCleanup: func(_ context.Context, env, stack string) error {
		*cleanups = append(*cleanups, destroyCleanup{env, stack})
		return nil
	}}
	if mode == "edge" {
		envID, apiURL = "example-storage", "edge://example-storage"
		registry = projectConflictTunnelRegistry(t, agent)
	} else {
		options.HTTPClient = &http.Client{Transport: destroyRoundTrip(func(r *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			agent.ServeHTTP(rec, r)
			return rec.Result(), nil
		})}
	}
	resolver := func(context.Context, string) (string, *string, bool, error) { return apiURL, nil, true, nil }
	auth := func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) {
		return authz.SudoPermissionSet(), &user.Actor{ID: "example-user", Username: "example-user"}, true
	}
	hub := echo.New()
	api := hub.Group("/api")
	api.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", resolver, auth, nil, registry, nil, options))
	api.Any("/environments/:id/*", func(c *echo.Context) error { return c.NoContent(http.StatusTeapot) })
	return envID, hub, cleanups
}

func destroyFixture(t *testing.T) (*ProjectService, string, string) {
	t.Helper()
	svc, db, projectsDir := setupProjectDestroyTestService(t)
	path := filepath.Join(projectsDir, "destroy-demo")
	require.NoError(t, os.MkdirAll(path, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "data.txt"), []byte("retained"), 0o600))
	require.NoError(t, db.Create(&Project{ID: "destroy-project", Name: "destroy-demo", Path: path, Status: ProjectStatusStopped}).Error)
	return svc, projectsDir, path
}

func sendDestroy(hub http.Handler, envID string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/api/environments/"+envID+"/projects/destroy-project/destroy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	hub.ServeHTTP(rec, req)
	return rec
}

func TestStackDestroyChainKeepsFilesUnlessExplicitlyRemoved(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		for _, body := range []string{"", `{}`, `{"removeFiles":false}`, `{"removeFiles":true}`} {
			t.Run(mode+"/"+body, func(t *testing.T) {
				svc, projectsDir, path := destroyFixture(t)
				envID, hub, cleanups := destroyChain(t, mode, projectConflictRouter(t, svc))
				rec := sendDestroy(hub, envID, []byte(body))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.NoDirExists(t, path)
				trashed, err := filepath.Glob(filepath.Join(projectsDir, ".arcane-trash-*", "data.txt"))
				require.NoError(t, err)
				if body == `{"removeFiles":true}` {
					require.Empty(t, trashed, "explicit removeFiles deletes the files")
				} else {
					require.Len(t, trashed, 1, "omitted or false removeFiles keeps the files")
					data, err := os.ReadFile(trashed[0])
					require.NoError(t, err)
					require.Equal(t, "retained", string(data))
				}
				if mode != "local" {
					require.Equal(t, []destroyCleanup{{envID, "destroy-project"}}, *cleanups)
				}
			})
		}
	}
}

func TestEdgeStackDestroyChainBoundsTheBody(t *testing.T) {
	explicit := `{"removeFiles":true}`
	padded := func(size int) []byte { return []byte(explicit + strings.Repeat(" ", size-len(explicit))) }
	t.Run("at the bound", func(t *testing.T) {
		svc, projectsDir, path := destroyFixture(t)
		envID, hub, _ := destroyChain(t, "edge", projectConflictRouter(t, svc))
		rec := sendDestroy(hub, envID, padded(64<<10))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.NoDirExists(t, path)
		trashed, err := filepath.Glob(filepath.Join(projectsDir, ".arcane-trash-*"))
		require.NoError(t, err)
		require.Empty(t, trashed, "the explicit flag survives the full-size body")
	})
	t.Run("over the bound", func(t *testing.T) {
		svc, _, path := destroyFixture(t)
		envID, hub, cleanups := destroyChain(t, "edge", projectConflictRouter(t, svc))
		rec := sendDestroy(hub, envID, padded(64<<10+1))
		require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
		require.Contains(t, rec.Body.String(), `"request.too-large"`)
		require.FileExists(t, filepath.Join(path, "data.txt"), "a refused request changes nothing")
		require.Empty(t, *cleanups)
	})
}

func TestStackDestroyOutsideProjectsDirectoryKeepsFilesInPlace(t *testing.T) {
	svc, db, _ := setupProjectDestroyTestService(t)
	path := t.TempDir()
	data := filepath.Join(path, "external.txt")
	require.NoError(t, os.WriteFile(data, []byte("external retained"), 0o600))
	require.NoError(t, db.Create(&Project{ID: "destroy-project", Name: "external-demo", Path: path, Status: ProjectStatusStopped}).Error)
	envID, hub, _ := destroyChain(t, "local", projectConflictRouter(t, svc))
	rec := sendDestroy(hub, envID, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	content, err := os.ReadFile(data)
	require.NoError(t, err)
	require.Equal(t, "external retained", string(content))
	var count int64
	require.NoError(t, db.Model(&Project{}).Where("id = ?", "destroy-project").Count(&count).Error)
	require.Zero(t, count)
}
