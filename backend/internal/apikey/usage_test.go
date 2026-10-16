// Source: getarcaneapp/arcane/backend/internal/apikey/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed template catalogue; functional cut: removed OIDC and passkeys; SQLite temp-file persistence fixture; usage-write checks split from the retained suite.

package apikey

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/apikey"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"runtime"
	"testing"
	"time"
)

func TestValidateAPIKeyDebouncesLastUsedWrites(t *testing.T) {
	ctx := t.Context()
	service, db, userService := setupAPIKeyService(t)
	localUser := createTestAPIKeyUser(t, ctx, userService, "user-debounce")

	created, err := service.CreateApiKey(ctx, localUser.ID, authz.SudoPermissionSet(), apikey.CreateApiKey{Name: "debounce-key"})
	require.NoError(t, err)

	_, err = service.ValidateApiKey(ctx, created.Key)
	require.NoError(t, err)
	require.NotNil(t, fetchAPIKey(t, db, created.ID).LastUsedAt)

	// Clear the column directly; a validation inside the debounce window must
	// not issue another write.
	require.NoError(t, db.Model(&ApiKey{}).Where("id = ?", created.ID).Update("last_used_at", nil).Error)

	_, err = service.ValidateApiKey(ctx, created.Key)
	require.NoError(t, err)
	require.Nil(t, fetchAPIKey(t, db, created.ID).LastUsedAt)
}

func TestValidateAPIKeyDebounceReleasedOnWriteFailure(t *testing.T) {
	ctx := t.Context()
	service, db, userService := setupAPIKeyService(t)
	localUser := createTestAPIKeyUser(t, ctx, userService, "user-debounce-retry")

	created, err := service.CreateApiKey(ctx, localUser.ID, authz.SudoPermissionSet(), apikey.CreateApiKey{Name: "debounce-retry-key"})
	require.NoError(t, err)

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	service.markApiKeyUsedDebouncedInternal(canceledCtx, created.ID)
	require.Nil(t, fetchAPIKey(t, db, created.ID).LastUsedAt)

	// The failed write must not leave the key debounced: the next use inside
	// the window must retry and persist.
	service.markApiKeyUsedDebouncedInternal(ctx, created.ID)
	require.NotNil(t, fetchAPIKey(t, db, created.ID).LastUsedAt)
}

func TestValidateAPIKeyLastUsedUpdateIsRequestScoped(t *testing.T) {
	ctx := t.Context()
	service, db, userService := setupAPIKeyService(t)
	localUser := createTestAPIKeyUser(t, ctx, userService, "user-request-scoped")

	created, err := service.CreateApiKey(ctx, localUser.ID, authz.SudoPermissionSet(), apikey.CreateApiKey{Name: "request-scoped-key"})
	require.NoError(t, err)

	updateStarted := make(chan struct{})
	releaseUpdate := make(chan struct{})
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("block_api_key_last_used_update", func(*gorm.DB) {
		close(updateStarted)
		<-releaseUpdate
	}))

	validationDone := make(chan error, 1)
	go func() {
		_, validateApiKeyErr := service.ValidateApiKey(ctx, created.Key)
		validationDone <- validateApiKeyErr
	}()

	select {
	case <-updateStarted:
	case <-time.After(time.Second):
		close(releaseUpdate)
		require.FailNow(t, "last-used update did not start")
	}

	select {
	case operationErr := <-validationDone:
		close(releaseUpdate)
		require.NoError(t, operationErr)
		require.FailNow(t, "validation returned before its last-used update completed")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseUpdate)
	require.NoError(t, <-validationDone)
}

func TestMarkAPIKeyUsedHonorsRequestCancellation(t *testing.T) {
	ctx := t.Context()
	service, db, userService := setupAPIKeyService(t)
	localUser := createTestAPIKeyUser(t, ctx, userService, "user-canceled-usage")

	created, err := service.CreateApiKey(ctx, localUser.ID, authz.SudoPermissionSet(), apikey.CreateApiKey{Name: "canceled-usage-key"})
	require.NoError(t, err)

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	err = service.markApiKeyUsedInternal(canceledCtx, created.ID)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, fetchAPIKey(t, db, created.ID).LastUsedAt)
}

func TestMarkAPIKeyUsedReturnsDatabaseErrors(t *testing.T) {
	ctx := t.Context()
	service, db, userService := setupAPIKeyService(t)
	localUser := createTestAPIKeyUser(t, ctx, userService, "user-usage-error")

	created, err := service.CreateApiKey(ctx, localUser.ID, authz.SudoPermissionSet(), apikey.CreateApiKey{Name: "usage-error-key"})
	require.NoError(t, err)

	sqlDB, err := db.DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	err = service.markApiKeyUsedInternal(ctx, created.ID)
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to update API key last-used timestamp")
}

func TestValidateAPIKeyRepeatedAuthenticationDoesNotGrowGoroutines(t *testing.T) {
	ctx := t.Context()
	service, _, userService := setupAPIKeyService(t)
	localUser := createTestAPIKeyUser(t, ctx, userService, "user-repeated-auth")

	created, err := service.CreateApiKey(ctx, localUser.ID, authz.SudoPermissionSet(), apikey.CreateApiKey{Name: "repeated-auth-key"})
	require.NoError(t, err)

	before := runtime.NumGoroutine()
	for range 20 {
		validatedUser, validateApiKeyErr := service.ValidateApiKey(ctx, created.Key)
		require.NoError(t, validateApiKeyErr)
		require.Equal(t, localUser.ID, validatedUser.ID)
	}
	// database/sql releases per-query cancellation watchers asynchronously.
	var after int
	require.Eventually(t, func() bool {
		after = runtime.NumGoroutine()
		return after <= before+1
	}, 2*time.Second, 10*time.Millisecond)
	t.Logf("goroutines before repeated authentication: %d; after drain: %d", before, after)
}
