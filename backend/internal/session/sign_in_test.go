// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original durable mode transaction failure proof.

package session

import (
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/types/auth"
	"github.com/stretchr/testify/require"
)

func TestSignInModeTransitionRollsBackOnFailure(t *testing.T) {
	db := setupAuthServiceTestDB(t)
	svc := NewSessionService(db)
	require.NoError(t, svc.ActivateSignInMode(t.Context(), auth.SignInModeLocal))
	require.NoError(t, db.Create(&user.User{ID: "admin", Username: "admin"}).Error)
	current, _, err := svc.CreateSession(t.Context(), "admin", time.Now().Add(time.Hour), auth.SessionMeta{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_mode_write BEFORE UPDATE ON settings WHEN NEW.key = 'authLastMode' BEGIN SELECT RAISE(ABORT, 'injected write failure'); END`).Error)
	require.Error(t, svc.ActivateSignInMode(t.Context(), auth.SignInModeProxy))
	current, err = svc.GetSessionByID(t.Context(), current.ID)
	require.NoError(t, err)
	require.NoError(t, ValidateActive(current))
	var previous settings.SettingVariable
	require.NoError(t, db.Where("key = ?", "authLastMode").First(&previous).Error)
	require.Equal(t, "local", previous.Value)
}
