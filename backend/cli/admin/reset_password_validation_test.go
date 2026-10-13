// Source: getarcaneapp/arcane/backend/cli/admin/reset_password_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: functional cut: password validation tests run without database integration.

package admin

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/validation"
)

func TestEnsurePasswordResetEnabledInternal(t *testing.T) {
	require.ErrorContains(t, ensurePasswordResetEnabledInternal(&config.Config{}), "ALLOW_CLI_PASSWORD_RESET=true")
	require.NoError(t, ensurePasswordResetEnabledInternal(&config.Config{AllowCLIPasswordReset: true}))
}

func TestValidatePasswordPairInternal(t *testing.T) {
	require.ErrorContains(t, validatePasswordPairInternal("short", "short", validation.PasswordPolicyBasic), "at least 8")
	require.ErrorContains(t, validatePasswordPairInternal("new-password", "different", validation.PasswordPolicyBasic), "do not match")
	require.NoError(t, validatePasswordPairInternal("new-password", "new-password", validation.PasswordPolicyBasic))
}
