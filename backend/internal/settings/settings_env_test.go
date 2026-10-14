//go:build postgres_integration

// Source: getarcaneapp/arcane/backend/internal/settings/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-14
// License: BSD-3-Clause
// Deviations: PostgreSQL initialization regression for invalid environment values.

package settings

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvalidEnvironmentOverrideDoesNotFailInitialization(t *testing.T) {
	db := setupSettingsTestDB(t)
	svc, err := newSettingsServiceForTestInternal(t, t.Context(), db)
	require.NoError(t, err)
	t.Setenv("UPGRADE_LOG_RETENTION_DAYS", "invalid")
	_, err = svc.loadDatabaseConfigFromEnv(t.Context(), db)
	require.NoError(t, err)
	require.NoError(t, svc.upsertEnvSetting(t.Context(), db.DB, "upgradeLogRetentionDays", "invalid"))
	require.Equal(t, 3, svc.GetIntSetting(t.Context(), "upgradeLogRetentionDays", 3))
}
