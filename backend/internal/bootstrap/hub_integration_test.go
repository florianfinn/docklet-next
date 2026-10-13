//go:build postgres_integration

// Source: docklet-next/docs/design/core-adoption.md
// Revision: be847e36ab81fdf0cd6a7dad333046ddea990e71
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Original issue #8 empty-database hub readiness test.

package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/config"
)

func TestHubStartsAgainstEmptyPostgres(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	require.NotEmpty(t, dsn)
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminSQL.Close() })
	var random [8]byte
	_, err = rand.Read(random[:])
	require.NoError(t, err)
	name := "docklet_hub_" + hex.EncodeToString(random[:])
	require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
	t.Cleanup(func() { require.NoError(t, admin.Exec("DROP DATABASE "+name+" WITH (FORCE)").Error) })
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.Path = "/" + name
	freshURL := u.String()
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
