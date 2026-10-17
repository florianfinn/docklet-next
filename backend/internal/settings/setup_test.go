// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original first-run password gate and hub settings SQLite restart tests.

package settings

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	settingstypes "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSetupCompletionRequiresChangedInitialPassword(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDB(t)
	require.NoError(t, db.Exec("CREATE TABLE users (id TEXT PRIMARY KEY, requires_password_change BOOLEAN NOT NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO users (id, requires_password_change) VALUES (?, ?)", "admin", true).Error)
	svc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)
	handler := &SettingsHandler{settingsService: svc}
	actor := context.WithValue(ctx, middleware.ContextKeyUserID, "admin")
	input := settingstypes.Update{SetupCompleted: new("true")}
	require.ErrorContains(t, handler.validateSettingsUpdateInput(actor, input), "Change the initial administrator password")
	require.NoError(t, db.Exec("UPDATE users SET requires_password_change = ? WHERE id = ?", false, "admin").Error)
	require.NoError(t, handler.validateSettingsUpdateInput(actor, input))
}

func TestSetupSettingsPersistAcrossServiceRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hub.db")
	open := func() *database.DB {
		gormDB, err := gorm.Open(testsqlite.Dialector(t, path), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, gormDB.AutoMigrate(&SettingVariable{}))
		return &database.DB{DB: gormDB}
	}
	first, err := NewSettingsService(t.Context(), open())
	require.NoError(t, err)
	_, err = first.UpdateSettings(t.Context(), settingstypes.Update{
		SetupCompleted: new("true"), HubName: new("Example Hub"),
		KeepActionOutput: new("false"), ActionPresentation: new("toast"),
	})
	require.NoError(t, err)
	require.NoError(t, first.Stop(context.Background()))
	second, err := NewSettingsService(t.Context(), open())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, second.Stop(context.Background())) })
	stored, err := second.GetSettings(t.Context())
	require.NoError(t, err)
	require.Equal(t, "true", stored.SetupCompleted.Value)
	require.Equal(t, "Example Hub", stored.HubName.Value)
	require.Equal(t, "false", stored.KeepActionOutput.Value)
	require.Equal(t, "toast", stored.ActionPresentation.Value)
}

func TestSetupDefaultHostDirectoriesDoNotTouchHubProjects(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission regression requires a non-root process")
	}
	hubProjects := filepath.Join(t.TempDir(), "hub-projects")
	require.NoError(t, os.Mkdir(hubProjects, 0700))
	t.Setenv("PROJECTS_DIRECTORY", hubProjects)
	svc, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	cfg := svc.GetSettingsConfig()
	require.Equal(t, hubProjects, cfg.ProjectsDirectory.Value)
	handler := &SettingsHandler{settingsService: svc, cfg: &config.Config{AuthMode: "none"}}
	// Matches the default web setup payload; agent paths must never be opened on the hub.
	payload := `{"hubName":"Docklet","hubAddress":"192.0.2.10:8080","domainUrl":"","linkBase":"auto","agentProjectsDirectory":"/home/docker","agentInstallDirectory":"/home/docker/docklet-agent","defaultHostConnection":"direct","defaultPuid":"","defaultPgid":"","detectSocketGroup":"true","setupCompleted":"true"}`
	var input settingstypes.Update
	require.NoError(t, json.Unmarshal([]byte(payload), &input))
	require.Nil(t, input.ProjectsDirectory)
	output, err := handler.UpdateSettings(t.Context(), &UpdateSettingsInput{EnvironmentID: "0", Body: input})
	require.NoError(t, err)
	require.True(t, output.Body.Success)
	stored := svc.GetSettingsConfig()
	require.Equal(t, hubProjects, stored.ProjectsDirectory.Value)
	require.Equal(t, "/home/docker", stored.AgentProjectsDirectory.Value)
	require.Equal(t, "/home/docker/docklet-agent", stored.AgentInstallDirectory.Value)
	require.Equal(t, "true", stored.SetupCompleted.Value)
}

func TestSetupCompletionSignInModeGate(t *testing.T) {
	for _, mode := range []string{"local", "none", "proxy"} {
		t.Run(mode, func(t *testing.T) {
			db := setupSettingsTestDB(t)
			require.NoError(t, db.Exec("CREATE TABLE users (id TEXT PRIMARY KEY, requires_password_change BOOLEAN NOT NULL)").Error)
			require.NoError(t, db.Exec("INSERT INTO users (id, requires_password_change) VALUES (?, ?)", "admin", true).Error)
			svc, err := newSettingsServiceForTestInternal(t, t.Context(), db)
			require.NoError(t, err)
			handler := &SettingsHandler{settingsService: svc, cfg: &config.Config{AuthMode: mode}}
			input := &UpdateSettingsInput{EnvironmentID: "0", Body: settingstypes.Update{SetupCompleted: new("true")}}
			actor := context.WithValue(t.Context(), middleware.ContextKeyUserID, "admin")
			_, err = handler.UpdateSettings(actor, input)
			if mode == "local" {
				require.ErrorContains(t, err, "Change the initial administrator password")
				_, err = handler.UpdateSettings(t.Context(), input)
				require.ErrorContains(t, err, "Sign-in required")
				require.NoError(t, db.Exec("UPDATE users SET requires_password_change = ?", false).Error)
				_, err = handler.UpdateSettings(actor, input)
				require.NoError(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSettingsReadEffectiveTimeZoneFromProcessTZ(t *testing.T) {
	for _, zone := range []string{"", "UTC", "Europe/Berlin", "America/New_York"} {
		t.Run(zone, func(t *testing.T) {
			t.Setenv("TZ", zone)
			cfg := config.Load()
			svc, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
			require.NoError(t, err)
			handler := &SettingsHandler{settingsService: svc, cfg: cfg}
			ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
			output, err := handler.GetSettings(ctx, &GetSettingsInput{EnvironmentID: "0"})
			require.NoError(t, err)
			values := runtimeSettingKeysInternal(output.Body)
			expected := zone
			if expected == "" {
				expected = "UTC"
			}
			require.Equal(t, expected, values["effectiveTimeZone"])
			require.Equal(t, cfg.GetLocation().String(), values["effectiveTimeZone"])
			require.NotContains(t, values, "timeZone")
		})
	}
}
