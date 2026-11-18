// Source: docklet-next/issues/518
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-18
// License: Apache-2.0
// Deviations: Original test seeding of a claimed first-run administrator.

package testfixtures

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/user"
)

// ClaimedAdministrator creates the first administrator through the claim path and marks setup as claimed,
// as first-run setup does, so InitializeSignIn starts outside the setup-pending state.
func ClaimedAdministrator(t *testing.T, db *gorm.DB, users *user.UserService, username, password string) *user.User {
	t.Helper()
	var admin *user.User
	require.NoError(t, db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		if admin, err = users.ClaimFirstAdministratorInDB(t.Context(), tx, user.FirstAdministrator{Username: username, Password: password}); err != nil {
			return err
		}
		return tx.Save(&settings.SettingVariable{Key: session.SettingSetupIdentity, Value: session.SetupIdentityDone}).Error
	}))
	users.AfterFirstAdministratorClaim(admin.ID)
	return admin
}

// MarkSetupClaimed records a completed first-run claim for fixtures that seed their administrator directly.
func MarkSetupClaimed(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.WithContext(t.Context()).Save(&settings.SettingVariable{Key: session.SettingSetupIdentity, Value: session.SetupIdentityDone}).Error)
}
