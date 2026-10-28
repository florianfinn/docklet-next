// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original real-route recovery-key permission, audit and verified attachment proofs.

package backup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	kit "go.getarcane.app/kit/pkg"
	"go.getarcane.app/sys/crypto"
)

func TestRecoveryKeyRoutesRequirePermissionAndRecordKeyFreeNoStoreAudit(t *testing.T) {
	for _, mode := range []string{"hub-target", "hub-local", "agent-local"} {
		for _, grant := range []string{"none", "backup", "restore", "admin"} {
			t.Run(mode+"/"+grant, func(t *testing.T) {
				f := newRuntimePlan(t)
				require.NoError(t, f.service.deps.DB.AutoMigrate(&event.Event{}, &backuptarget.Target{}, &backuptarget.Check{}))
				crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
				key, err := backup.GenerateRecoveryKey()
				require.NoError(t, err)
				encrypted, err := crypto.Encrypt(key)
				require.NoError(t, err)
				require.NoError(t, f.service.deps.DB.Create(&backuptarget.Target{ID: "archive", Kind: "smb", Name: "Archive", RecoveryKeyEnc: encrypted, ConfigVersion: 1, CreatedAt: time.Now().UTC()}).Error)
				f.service.deps.EncryptionKey = "synthetic-local-key-for-derivation"
				f.service.deps.Events = event.NewEventService(f.service.deps.DB, nil, nil)
				ps := authz.NewPermissionSet()
				switch grant {
				case "backup":
					ps.AddEnv("0", authz.PermVolumesBackup)
				case "restore":
					ps.AddGlobal(authz.PermSystemBackupsRestore)
				case "admin":
					ps = authz.SudoPermissionSet()
				}
				router, api := humatest.New(t)
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					request := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, ps)
					request = context.WithValue(request, user.CurrentUserContextKey{}, &user.SystemUser)
					next(huma.WithContext(ctx, request))
				})
				cfg := &config.Config{AgentMode: mode == "agent-local"}
				handler := TargetHandler(f.service, nil, cfg, t.Context())
				path := "/environments/0/volumes/backups/targets/local/recovery-key"
				allowed := grant == "admin" || (mode == "hub-local" && grant == "restore") || (mode == "agent-local" && grant == "backup")
				if mode == "hub-target" {
					backuptarget.RegisterHubRoutes(api, handler)
					path = "/backup-targets/archive/recovery-key"
				} else {
					backuptarget.RegisterHostRoutes(api, handler, cfg.AgentMode)
					key = kit.SHA256Hex(legacyVolumePasswordSaltInternal + f.service.deps.EncryptionKey)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
				var events []event.Event
				require.NoError(t, f.service.deps.DB.Find(&events).Error)
				if !allowed {
					require.Equal(t, 403, response.Code)
					require.NotContains(t, response.Body.String(), key)
					require.Empty(t, events)
					return
				}
				require.Equal(t, 200, response.Code, response.Body.String())
				require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
				require.Contains(t, response.Body.String(), key)
				require.Len(t, events, 1)
				require.Equal(t, event.EventTypeVolumeBackupTargetKeyViewed, events[0].Type)
				require.Equal(t, "volume.backup.target.key_viewed", events[0].MessageCode)
				audit, err := json.Marshal(events[0])
				require.NoError(t, err)
				require.NotContains(t, string(audit), key)
				require.NotContains(t, string(audit), encrypted)
				require.Empty(t, f.creates, "viewing a key must not open the repository")
			})
		}
	}
}
func TestLocalDerivedKeyAndVerifiedAttachNeverRekeyRepository(t *testing.T) {
	f := newRuntimePlan(t)
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
	require.NoError(t, f.service.deps.DB.AutoMigrate(&backup.SystemBackupRecoveryConfig{}))
	f.service.deps.RecoveryKeys = backup.NewRecoveryKeyStore(f.service.deps.DB)
	f.service.deps.EncryptionKey = "first-synthetic-host-key"
	handler := TargetHandler(f.service, nil, nil, t.Context())
	first, err := handler.LocalKey(t.Context())
	require.NoError(t, err)
	require.Equal(t, kit.SHA256Hex(legacyVolumePasswordSaltInternal+f.service.deps.EncryptionKey), first)
	require.Len(t, first, 64)
	f.service.deps.EncryptionKey = "second-synthetic-host-key"
	second, err := handler.LocalKey(t.Context())
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	// Fail before verifying: the saved password must remain absent.
	f.exit = 1
	f.stdout = ""
	f.stderr = "No suitable key found for the given password."
	err = handler.AttachLocalKey(t.Context(), first)
	require.ErrorContains(t, err, "backup.target.key-mismatch")
	configured, err := f.service.deps.RecoveryKeys.Configured(t.Context())
	require.NoError(t, err)
	require.False(t, configured)
	f.exit = 0
	f.stdout = "[]"
	f.stderr = ""
	require.NoError(t, handler.AttachLocalKey(t.Context(), first))
	stored, err := handler.LocalKey(t.Context())
	require.NoError(t, err)
	require.Equal(t, first, stored)
	f.service.deps.EncryptionKey = "third-synthetic-host-key"
	stored, err = handler.LocalKey(t.Context())
	require.NoError(t, err)
	require.Equal(t, first, stored)
	require.Len(t, f.creates, 2)
	for _, created := range f.creates {
		require.Equal(t, []string{"snapshots", "--json"}, created.Cmd)
		require.NotContains(t, strings.Join(created.Cmd, " "), "key")
	}
	// Repositories are read using the submitted key, never an empty/generated one.
	for _, created := range f.creates {
		require.Contains(t, created.Env, "RUSTIC_PASSWORD="+first)
	}
}
