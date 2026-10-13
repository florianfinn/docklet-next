// Source: getarcaneapp/arcane/backend/cli/generate/api_key_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package generate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyCommandAvailableInBackendCLI(t *testing.T) {
	cmd := GenerateCmd
	cmd.SetArgs([]string{"api-key"})

	out, err := captureOutput(func() error {
		_, err := cmd.ExecuteC()
		return err
	})

	require.NoError(t, err,
		"command failed: %v", err)

	require.NotEmpty(t, strings.TrimSpace(out),
		"expected raw arc_ key in output, got: %q", out)

	require.True(t, strings.HasPrefix(strings.TrimSpace(out), "arc_"),
		"expected raw arc_ key in output, got: %q", out)

	require.NotContains(t, out, "ADMIN_STATIC_API_KEY",
		"expected raw arc_ key in output, got: %q", out)
}
