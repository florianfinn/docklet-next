// Source: getarcaneapp/arcane/backend/pkg/utils/francis/provider_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: functional cut: removed SQLite actor storage tests; functional cut: removed SQLite runtime and migrations.

package francis

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStoreURLRequiresPostgres(t *testing.T) {
	for _, input := range []string{"postgres://docklet@example.invalid/docklet", "postgresql://docklet@example.invalid/docklet"} {
		actual, err := StoreURL(input)
		require.NoError(t, err)
		require.Equal(t, input, actual)
	}
	for _, input := range []string{"file:legacy.db", "mysql://localhost/test", ""} {
		_, err := StoreURL(input)
		require.Error(t, err)
	}
}
