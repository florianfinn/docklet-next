// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package di

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/stretchr/testify/require"
)

func TestStackImportProviderUsesConfiguredMirror(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if strings.HasSuffix(r.URL.Path, ".env.example") {
			w.WriteHeader(404)
			return
		}
		_, _ = io.WriteString(w, "services:\n  app:\n    image: alpine:3\n")
	}))
	defer server.Close()
	module, err := provideStackImportModuleInternal(&config.Config{StackImportGitHubRawURL: server.URL}, nil)
	require.NoError(t, err)
	_, api := humatest.New(t)
	api.UseMiddleware(func(c huma.Context, next func(huma.Context)) {
		ctx := context.WithValue(c.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
		next(huma.WithContext(c, context.WithValue(ctx, middleware.ContextKeyUserID, "admin")))
	})
	module.RegisterRoutes(api)
	response := api.Post("/stack-import", map[string]any{"kind": "github", "ref": "owner/repo"})
	require.Equal(t, 200, response.Code)
	require.Contains(t, response.Body.String(), "import.found.compose")
	require.Equal(t, int32(2), calls.Load())
	_, err = provideStackImportModuleInternal(&config.Config{StackImportGitHubRawURL: "http://mirror.example.com"}, nil)
	require.Error(t, err)
}

// Agents never serve the import route, so an invalid mirror setting does not stop them.
func TestStackImportProviderSkipsMirrorOnAgents(t *testing.T) {
	for _, cfg := range []*config.Config{{AgentMode: true, StackImportGitHubRawURL: "http://mirror.example.com"}, {EdgeAgent: true, StackImportGitHubRawURL: "http://mirror.example.com"}} {
		module, err := provideStackImportModuleInternal(cfg, nil)
		require.NoError(t, err)
		require.NotNil(t, module)
	}
}
