// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original actual sync-route old-token commit refusal after pairing rotation.

package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

func TestSyncRouteChecksCurrentPairingTokenAfterEarlierAuthentication(t *testing.T) {
	f := newRuntimePlan(t)
	require.NoError(t, f.service.deps.DB.AutoMigrate(&backuptarget.Target{}, &backuptarget.Check{}))
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
	oldToken, newToken := strings.Repeat("o", 32), strings.Repeat("n", 32)
	require.NoError(t, f.service.deps.DB.Exec("INSERT INTO settings(key,value) VALUES ('agentToken',?)", oldToken).Error)
	key, err := backup.GenerateRecoveryKey()
	require.NoError(t, err)
	host := "host-a"
	targets := []dto.SyncedTarget{{ID: "archive", Name: "Archive", TargetLocation: dto.TargetLocation{Kind: "local", HostID: &host, Path: "/srv/backups"}, RecoveryKey: key, ConfigVersion: 1}}
	hash, err := backuptarget.ContentHash(targets)
	require.NoError(t, err)
	set := dto.TargetSet{Authority: "authority", HubHostID: host, SetVersion: 1, ContentHash: hash, Targets: targets}
	body, err := json.Marshal(set)
	require.NoError(t, err)
	router, api := humatest.New(t)
	rotate := true
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		// Authentication already accepted the old token. A new pairing commits
		// before target application begins; the transaction must re-read its token.
		if rotate {
			require.NoError(t, f.service.deps.DB.Exec("UPDATE settings SET value=? WHERE key='agentToken'", newToken).Error)
			rotate = false
		}
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	backuptarget.RegisterHostRoutes(api, TargetHandler(f.service, nil, &config.Config{AgentMode: true, AgentToken: oldToken}, t.Context()), true)
	request := func(token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPut, "/environments/0/volumes/backups/targets", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Arcane-Agent-Token", token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, r)
		return response
	}
	refused := request(oldToken)
	require.GreaterOrEqual(t, refused.Code, 400, "old authenticated request cannot commit a target set")
	var count int64
	require.NoError(t, f.service.deps.DB.Model(&backuptarget.Target{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, f.service.deps.DB.Table("settings").Where("key LIKE ?", "backupTarget%").Count(&count).Error)
	require.Zero(t, count, "transaction rolls back set metadata")
	accepted := request(newToken)
	require.Equal(t, 200, accepted.Code, accepted.Body.String())
	require.NoError(t, f.service.deps.DB.Model(&backuptarget.Target{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.Empty(t, f.creates, "metadata sync never mounts a repository")
}

func TestSyncRouteUsesConfiguredTokenWhenStoredPairingTokenIsEmpty(t *testing.T) {
	f := newRuntimePlan(t)
	require.NoError(t, f.service.deps.DB.AutoMigrate(&backuptarget.Target{}, &backuptarget.Check{}))
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
	configured := strings.Repeat("c", 32)
	// Default settings seed an empty agentToken row on agents configured through the environment.
	require.NoError(t, f.service.deps.DB.Exec("INSERT INTO settings(key,value) VALUES ('agentToken','')").Error)
	key, err := backup.GenerateRecoveryKey()
	require.NoError(t, err)
	host := "host-a"
	targets := []dto.SyncedTarget{{ID: "archive", Name: "Archive", TargetLocation: dto.TargetLocation{Kind: "local", HostID: &host, Path: "/srv/backups"}, RecoveryKey: key, ConfigVersion: 1}}
	hash, err := backuptarget.ContentHash(targets)
	require.NoError(t, err)
	body, err := json.Marshal(dto.TargetSet{Authority: "authority", HubHostID: host, SetVersion: 1, ContentHash: hash, Targets: targets})
	require.NoError(t, err)
	router, api := humatest.New(t)
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	backuptarget.RegisterHostRoutes(api, TargetHandler(f.service, nil, &config.Config{AgentMode: true, AgentToken: configured}, t.Context()), true)
	request := func(token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPut, "/environments/0/volumes/backups/targets", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Arcane-Agent-Token", token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, r)
		return response
	}
	refused := request(strings.Repeat("x", 32))
	require.GreaterOrEqual(t, refused.Code, 400, "a token other than the configured one stays refused")
	accepted := request(configured)
	require.Equal(t, 200, accepted.Code, accepted.Body.String())
	var count int64
	require.NoError(t, f.service.deps.DB.Model(&backuptarget.Target{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
