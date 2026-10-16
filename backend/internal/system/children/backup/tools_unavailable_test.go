// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original client-tool availability regression tests for issue #32.

package backup

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"
)

func TestClientToolErrorsPrecedeStorageForActions(t *testing.T) {
	for _, tc := range []struct {
		name, dumpMajor, restoreMajor string
		want                          error
	}{
		{name: "both absent", want: ErrSystemBackupToolsUnavailable},
		{name: "dump absent", restoreMajor: "18", want: ErrSystemBackupToolsUnavailable},
		{name: "restore absent", dumpMajor: "18", want: ErrSystemBackupToolsUnavailable},
		{name: "dump wrong major", dumpMajor: "16", restoreMajor: "18", want: ErrSystemBackupToolsUnavailable},
		{name: "restore wrong major", dumpMajor: "18", restoreMajor: "16", want: ErrSystemBackupToolsUnavailable},
		{name: "tools available storage missing", dumpMajor: "18", restoreMajor: "18", want: ErrSystemBackupUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeClientTools(t, tc.dumpMajor, tc.restoreMajor)
			blocked := filepath.Join(t.TempDir(), "file")
			require.NoError(t, os.WriteFile(blocked, []byte("x"), 0600))
			cfg := &config.Config{DatabaseURL: "postgres://example:example@db.example.com/example", BackupDir: filepath.Join(blocked, "backups")}
			service := &Service{config: cfg, db: &database.DB{}, activityService: &activity.ActivityService{},
				maintenance: NewMaintenanceGate(), store: Store{
					CreateRun: func(context.Context, *backuptypes.SystemBackupRun) error { return nil },
					ReconcilePolicies: func(ctx context.Context, updates []backuptypes.UpdateSystemBackupPolicy,
						build func(context.Context, *backuptypes.SystemBackupPolicy, backuptypes.UpdateSystemBackupPolicy) error,
						_ func(context.Context, string), _ func(context.Context, *backuptypes.SystemBackupPolicy)) error {
						return build(ctx, &backuptypes.SystemBackupPolicy{}, updates[0])
					},
				}}
			_, err := service.StartBackup(t.Context(), usertypes.Actor{}, backuptypes.CreateSystemBackupRequest{})
			require.ErrorIs(t, err, tc.want)
			_, err = service.UpdatePolicies(t.Context(), []backuptypes.UpdateSystemBackupPolicy{{
				Enabled: true, Schedule: "0 0 3 * * *", RetentionCount: 1,
			}})
			require.ErrorIs(t, err, tc.want)
			_, err = service.UploadArchive(t.Context(), usertypes.Actor{}, bytes.NewBufferString("PGDMP"))
			require.ErrorIs(t, err, tc.want)
			_, err = service.StartRestore(t.Context(), usertypes.Actor{}, "", "archive-1", "archive-1")
			require.ErrorIs(t, err, tc.want)
			if tc.want == ErrSystemBackupToolsUnavailable {
				require.ErrorIs(t, OfflineRestore(t.Context(), cfg, "archive-1"), tc.want)
			}
			status := service.Storage()
			require.False(t, status.Available)
			require.Equal(t, tc.want.Error(), status.ReasonCode)
		})
	}
}

func TestClientToolErrorsUseServiceUnavailableResponses(t *testing.T) {
	for _, tc := range []struct {
		name, dumpMajor, restoreMajor string
	}{
		{name: "absent"},
		{name: "dump wrong major", dumpMajor: "16", restoreMajor: "18"},
		{name: "restore wrong major", dumpMajor: "18", restoreMajor: "16"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeClientTools(t, tc.dumpMajor, tc.restoreMajor)
			service := &Service{config: &config.Config{DatabaseURL: "postgres://example:example@db.example.com/example", BackupDir: t.TempDir()},
				db: &database.DB{}, activityService: &activity.ActivityService{}, maintenance: NewMaintenanceGate(),
				store: Store{CreateRun: func(context.Context, *backuptypes.SystemBackupRun) error { return nil }}}
			handler := NewHandler(service, nil, t.Context())
			_, err := handler.Create(t.Context(), &CreateSystemBackupInput{})
			assertToolsUnavailableResponse(t, err)
			_, err = handler.Restore(t.Context(), &RestoreSystemBackupInput{ID: "archive-1", Body: backuptypes.RestoreSystemBackupRequest{ConfirmBackupID: "archive-1"}})
			assertToolsUnavailableResponse(t, err)

			router := echo.New()
			api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("test", "1.0.0"))
			api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
				next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
			})
			RegisterRoutes(api, handler)
			req := httptest.NewRequest(http.MethodPost, "/api/backups/upload", bytes.NewBufferString("PGDMP"))
			req.Header.Set("Content-Type", "application/octet-stream")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "backup.tools_unavailable")
		})
	}
}

func assertToolsUnavailableResponse(t *testing.T, err error) {
	t.Helper()
	var statusErr huma.StatusError
	require.ErrorAs(t, err, &statusErr)
	require.Equal(t, http.StatusServiceUnavailable, statusErr.GetStatus())
	require.Contains(t, err.Error(), "backup.tools_unavailable")
}

func fakeClientTools(t *testing.T, dumpMajor, restoreMajor string) {
	t.Helper()
	dir := t.TempDir()
	for name, major := range map[string]string{"pg_dump": dumpMajor, "pg_restore": restoreMajor} {
		if major == "" {
			continue
		}
		script := "#!/bin/sh\nprintf '" + name + " (PostgreSQL) " + major + ".6\\n'\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(script), 0700))
	}
	t.Setenv("PATH", dir)
}
