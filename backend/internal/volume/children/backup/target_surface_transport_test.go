// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original target reference, immutability and key attachment HTTP/Edge proofs.

package backup

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

func TestTargetReferencesAndLocalKeyAttachThroughAllTransports(t *testing.T) {
	for _, mode := range []string{"hub", "direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			f := newRuntimePlan(t)
			db := f.service.deps.DB
			require.NoError(t, db.AutoMigrate(&backup.SystemBackupRecoveryConfig{}, &backuptarget.Target{}, &backuptarget.Check{}, &policies.VolumeBackupPolicy{}, &event.Event{}))
			crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
			f.service.deps.RecoveryKeys = backup.NewRecoveryKeyStore(db)
			f.service.deps.EncryptionKey = "synthetic-local-derivation-input"
			f.service.deps.Events = event.NewEventService(db, nil, nil)
			require.NoError(t, db.Create(&policies.VolumeBackupPolicy{VolumeName: "planned", TargetID: "archive"}).Error)
			for _, row := range []volume.Backup{{ID: "saved", VolumeName: "retained", Destination: "archive", Status: "succeeded"}, {ID: "running", VolumeName: "running", Destination: "archive", Status: "running"}} {
				require.NoError(t, db.Table("volume_backups").Create(&row).Error)
			}
			router, rawAPI := humatest.New(t)
			api := huma.NewGroup(rawAPI, "/api")
			api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
				request := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
				request = context.WithValue(request, user.CurrentUserContextKey{}, &user.SystemUser)
				next(huma.WithContext(ctx, request))
			})
			cfg := &config.Config{AgentMode: mode == "direct", EdgeAgent: mode == "edge"}
			backuptarget.RegisterHostRoutes(api, TargetHandler(f.service, nil, cfg, t.Context()), mode != "hub")
			call := backupRouteTransport(t, mode, router)
			status, _, body := call(t.Context(), "GET", "/api/environments/0/volumes/backups/targets/archive/references", nil)
			require.Equal(t, 200, status, string(body))
			require.Contains(t, string(body), `"policies":["planned"]`)
			require.Contains(t, string(body), `"running":["running"]`)
			require.Contains(t, string(body), `"retained"`)
			require.Empty(t, f.creates, "reference reads must use host metadata only")
			status, headers, body := call(t.Context(), "GET", "/api/environments/0/volumes/backups/targets/local/recovery-key", nil)
			require.Equal(t, 200, status, string(body))
			require.Equal(t, "no-store", headers.Get("Cache-Control"))
			var view struct {
				Data dto.TargetRecoveryKey `json:"data"`
			}
			require.NoError(t, json.Unmarshal(body, &view))
			key := view.Data.RecoveryKey
			require.Len(t, key, 64)
			payload, err := json.Marshal(dto.TargetRecoveryKey{RecoveryKey: key})
			require.NoError(t, err)
			f.stdout = ""
			f.stderr = "No suitable key found for the given password."
			f.exit = 1
			path := "/api/environments/0/volumes/backups/targets/local/recovery-key"
			status, _, body = call(t.Context(), "PUT", path, payload)
			require.Equal(t, 422, status, string(body))
			require.Contains(t, string(body), "backup.target.key-mismatch")
			require.NotContains(t, string(body), key)
			configured, err := f.service.deps.RecoveryKeys.Configured(t.Context())
			require.NoError(t, err)
			require.False(t, configured)
			f.stdout = "[]"
			f.stderr = ""
			f.exit = 0
			status, _, body = call(t.Context(), "PUT", path, payload)
			require.Equal(t, 200, status, string(body))
			f.service.deps.EncryptionKey = "changed-synthetic-host-key"
			status, _, body = call(t.Context(), "GET", path, nil)
			require.Equal(t, 200, status, string(body))
			require.Contains(t, string(body), key)
			require.Len(t, f.creates, 2)
			for _, created := range f.creates {
				require.Equal(t, []string{"snapshots", "--json"}, created.Cmd)
				require.Contains(t, created.Env, "RUSTIC_PASSWORD="+key)
			}
			var events []event.Event
			require.NoError(t, db.Find(&events).Error)
			audit, err := json.Marshal(events)
			require.NoError(t, err)
			require.NotContains(t, string(audit), key)
		})
	}
}

func TestHubTargetLocationEditsRefusedByRegisteredRoute(t *testing.T) {
	f := newRuntimePlan(t)
	require.NoError(t, f.service.deps.DB.AutoMigrate(&backuptarget.Target{}, &backuptarget.Check{}))
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
	input := dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "smb", Server: "nas.example.com", Share: "archive", Path: "original"}, Name: "Archive", Secret: new(strings.Repeat("s", 32))}
	row, err := f.service.targets.Create(t.Context(), input)
	require.NoError(t, err)
	router, api := humatest.New(t)
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	backuptarget.RegisterHubRoutes(api, TargetHandler(f.service, nil, nil, t.Context()))
	for _, field := range []string{"kind", "server", "share", "path"} {
		update := input
		switch field {
		case "kind":
			update.Kind = "nfs"
			update.Secret = nil
			update.Share = "/archive"
		case "server":
			update.Server = "other.example.com"
		case "share":
			update.Share = "different"
		case "path":
			update.Path = "different"
		}
		payload, err := json.Marshal(update)
		require.NoError(t, err)
		status, _, body := backupRouteTransport(t, "hub", router)(t.Context(), "PUT", "/backup-targets/"+row.ID, payload)
		require.Equal(t, 422, status, string(body))
		require.Contains(t, string(body), "backup.target.immutable")
		require.NotContains(t, string(body), *input.Secret)
	}
	unchanged, err := f.service.targets.Get(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, input.TargetLocation, unchanged.TargetLocation)
	require.Empty(t, f.creates)
}
