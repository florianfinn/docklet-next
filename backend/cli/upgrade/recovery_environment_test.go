// Source: getarcaneapp/arcane/backend/cli/upgrade/recovery_environment_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package upgrade

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyRecoveredEnvironmentInternal(t *testing.T) {
	result := applyRecoveredEnvironmentInternal([]string{
		"JWT_SECRET=fresh", "JWT_SECRET_FILE=/run/secrets/jwt", "ENCRYPTION_KEY__FILE=/run/secrets/key", "KEEP=value",
	}, map[string]string{"JWT_SECRET": "recovered-jwt", "ENCRYPTION_KEY": "recovered-key", "NEW_SETTING": "new"})

	require.Contains(t, result, "JWT_SECRET=recovered-jwt")
	require.Contains(t, result, "ENCRYPTION_KEY=recovered-key")
	require.Contains(t, result, "NEW_SETTING=new")
	require.Contains(t, result, "KEEP=value")
	require.False(t, slices.ContainsFunc(result, func(value string) bool {
		return value == "JWT_SECRET_FILE=/run/secrets/jwt" || value == "ENCRYPTION_KEY__FILE=/run/secrets/key"
	}))
}
