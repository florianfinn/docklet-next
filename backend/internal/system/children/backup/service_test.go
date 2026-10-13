// Source: getarcaneapp/arcane/backend/internal/system/children/backup/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed SQLite-only system recovery tests; functional cut: removed SQLite runtime and migrations.

package backup

import (
	"testing"

	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
)

func TestSystemRecoveryUnavailableUntilPostgresSupport(t *testing.T) {
	service := &Service{}
	require.False(t, service.SupportedDatabaseProvider())
	_, err := service.prepareBackupInternal(t.Context(), "manual", backuptypes.CreateSystemBackupRequest{})
	require.ErrorIs(t, err, ErrSystemBackupUnavailable)
}
