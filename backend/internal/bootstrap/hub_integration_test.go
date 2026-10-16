// Source: docklet-next/docs/design/core-adoption.md
// Revision: be847e36ab81fdf0cd6a7dad333046ddea990e71
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Original issue #8 empty-database hub readiness test.

package bootstrap

import (
	"context"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/florianfinn/docklet-next/backend/internal/config"
)

func TestHubStartsAgainstEmptySQLite(t *testing.T) {
	freshURL := sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db"))
	cfg := &config.Config{
		DatabaseURL:       freshURL,
		Environment:       config.AppEnvironmentTest,
		AppUrl:            "http://127.0.0.1",
		Listen:            "127.0.0.1",
		Port:              "0",
		DockerHost:        "unix:///nonexistent-docker.sock",
		ProjectsDirectory: t.TempDir(),
		EncryptionKey:     "test-only-encryption-key-32-chars",
	}
	db, err := initializeDBAndMigrateInternal(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	appCtx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	var server *http.Server
	app := fx.New(applicationOptions(appCtx, cfg, db, cancel), fx.Populate(&server))
	require.NoError(t, app.Err())
	startCtx, stopStart := context.WithTimeout(t.Context(), 30*time.Second)
	defer stopStart()
	require.NoError(t, app.Start(startCtx))
	t.Cleanup(func() {
		ctx, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		require.NoError(t, app.Stop(ctx))
	})
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + server.Addr + "/api/health")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
