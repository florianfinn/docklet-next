// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original first-run password gate and hub settings SQLite restart tests.

package settings

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
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
		SetupCompleted: new("true"), HubName: new("Example Hub"), TimeZone: new("UTC"),
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
	require.Equal(t, "UTC", stored.TimeZone.Value)
	require.Equal(t, "false", stored.KeepActionOutput.Value)
	require.Equal(t, "toast", stored.ActionPresentation.Value)
}
