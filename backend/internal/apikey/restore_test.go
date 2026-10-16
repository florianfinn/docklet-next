// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original coordinated SQLite backup and paired recovery for issue #32.

package apikey

import (
	apikeytypes "github.com/florianfinn/docklet-next/types/apikey"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRestoreInvalidatesCachedKeysAndRacingFills(t *testing.T) {
	service, db, users := setupAPIKeyService(t)
	user := createTestAPIKeyUser(t, t.Context(), users, "restore-key-user")
	created, err := service.CreatePersonalApiKey(t.Context(), user.ID, apikeytypes.CreateUserApiKey{Name: "restore-example"})
	require.NoError(t, err)
	_, err = service.ValidateApiKey(t.Context(), created.Key)
	require.NoError(t, err)
	generation := service.cacheGen.Load()
	row := fetchAPIKey(t, db, created.ID)
	require.NoError(t, db.Delete(&ApiKey{}, "id=?", created.ID).Error)
	service.InvalidateAfterRestore()
	service.storeValidatedKeyInternal(generation, service.hashRawAPIKeyInternal(created.Key), row)
	_, err = service.ValidateApiKey(t.Context(), created.Key)
	require.ErrorIs(t, err, ErrApiKeyInvalid)
	require.Empty(t, service.lastUsedWrites)
}
