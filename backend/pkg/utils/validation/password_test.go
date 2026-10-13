// Source: getarcaneapp/arcane/backend/pkg/utils/validation/password_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatePasswordPolicy(t *testing.T) {
	t.Parallel()

	require.Equal(t, "urn:arcane:problem:password-policy:basic", PasswordPolicyProblemType(PasswordPolicyBasic))
	require.Equal(t, "urn:arcane:problem:password-policy:standard", PasswordPolicyProblemType(PasswordPolicyStandard))
	require.Equal(t, "urn:arcane:problem:password-policy:strong", PasswordPolicyProblemType(PasswordPolicyStrong))
	require.Equal(t, "urn:arcane:problem:password-policy:strong", PasswordPolicyProblemType("bogus-policy"))

	require.NoError(t, ValidatePasswordPolicy("12345678", PasswordPolicyBasic))
	require.Error(t, ValidatePasswordPolicy("1234567", PasswordPolicyBasic))

	require.NoError(t, ValidatePasswordPolicy("Abcdefghi1", PasswordPolicyStandard))
	require.Error(t, ValidatePasswordPolicy("abcdefghij1", PasswordPolicyStandard))

	require.NoError(t, ValidatePasswordPolicy("Abcdefghij1!", PasswordPolicyStrong))
	require.Error(t, ValidatePasswordPolicy("Abcdefghijk1", PasswordPolicyStrong))
	require.Error(t, ValidatePasswordPolicy("Abcdefgh1!", "bogus-policy"))
}
