// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user schema and lifecycle regression tests.

package user_test

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestM15AssignmentSchema(t *testing.T) {
	db := setupAuthServiceTestDBInternal(t)
	var names []string
	require.NoError(t, db.Raw("SELECT name FROM sqlite_master WHERE type = 'table'").Scan(&names).Error)
	require.Contains(t, names, "user_app_assignments")
}
