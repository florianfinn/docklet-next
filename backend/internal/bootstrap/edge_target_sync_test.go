// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original real Edge-connect target delivery and non-blocking failure proof.

package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
	"go.uber.org/fx/fxtest"
)

func TestEdgeConnectDeliversTargetsAfterCompatibilityAndRegistry(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	defer cancel()
	cfg := &config.Config{DatabaseURL: sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db"))}
	db, err := initializeDBAndMigrateInternal(ctx, cfg)
	require.NoError(t, err)
	defer db.Close()
	token := "example"
	require.NoError(t, db.Create(&apikey.ApiKey{BaseModel: database.BaseModel{ID: "example-key"}, Name: "Example", KeyHash: "synthetic", KeyPrefix: "example", Kind: apikey.ApiKeyKindScoped}).Error)
	env := &environment.Environment{BaseModel: database.BaseModel{ID: "edge-connect"}, Name: "Example", ApiUrl: "edge://example", Enabled: true, IsEdge: true, AccessToken: &token, ApiKeyID: new("example-key")}
	require.NoError(t, db.Create(env).Error)
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
	encrypted, err := crypto.Encrypt("example")
	require.NoError(t, err)
	require.NoError(t, db.Create(&registry.ContainerRegistry{BaseModel: database.BaseModel{ID: "example-registry"}, URL: "registry.example.com", Username: "example", Token: encrypted, Enabled: true, RegistryType: registry.RegistryTypeGeneric}).Error)
	svc := environment.NewEnvironmentService(db, nil, nil, nil, nil, nil)
	router := echo.New()
	registry := edge.GetRegistry()
	registerEdgeTunnelRoutes(ctx, fxtest.NewLifecycle(t), cfg, router.Group("/api"), svc, nil, nil, registry)
	server := httptest.NewServer(router)
	defer server.Close()
	conn, _, err := websocket.Dial(ctx, edge.HTTPToWebSocketURL(server.URL)+"/api/tunnel/connect", &websocket.DialOptions{HTTPHeader: http.Header{edge.HeaderAgentToken: {token}}})
	require.NoError(t, err)
	defer conn.CloseNow()
	defer registry.Unregister(env.ID)
	require.NoError(t, wsjson.Write(ctx, conn, &edge.TunnelMessage{Type: edge.MessageTypeRegister, AgentToken: token, AgentInstance: "example-instance"}))
	var registered edge.TunnelMessage
	require.NoError(t, wsjson.Read(ctx, conn, &registered))
	require.True(t, registered.Accepted, "delivery must not block tunnel registration")
	paths := []string{}
	for {
		var request edge.TunnelMessage
		require.NoError(t, wsjson.Read(ctx, conn, &request))
		require.Equal(t, edge.MessageTypeCommandRequest, request.Type)
		paths = append(paths, request.Path)
		status := 200
		body := []byte(`{"success":true}`)
		switch request.Path {
		case "/api/app-version":
			body, err = json.Marshal(version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
			require.NoError(t, err)
		case "/api/environments/0/system/health":
		case "/api/container-registries/sync":
		case "/api/environments/0/volumes/backups/targets":
			require.Contains(t, paths, "/api/app-version")
			require.Contains(t, paths, "/api/container-registries/sync")
			require.Equal(t, http.MethodPut, request.Method)
			// A rejected sync must leave the connected tunnel usable.
			status = 503
			body = []byte(`{"code":"backup.target.sync-failed"}`)
		default:
			t.Fatalf("unexpected connect trigger %s", request.Path)
		}
		require.NoError(t, wsjson.Write(ctx, conn, &edge.TunnelMessage{ID: request.ID, Type: edge.MessageTypeResponse, Status: status, Body: body}))
		if status == 503 {
			break
		}
	}
	require.NoError(t, wsjson.Write(ctx, conn, &edge.TunnelMessage{ID: "after-sync", Type: edge.MessageTypeHeartbeat}))
	var ack edge.TunnelMessage
	require.NoError(t, wsjson.Read(ctx, conn, &ack))
	require.Equal(t, edge.MessageTypeHeartbeatAck, ack.Type)
	require.Equal(t, "after-sync", ack.ID)
}
