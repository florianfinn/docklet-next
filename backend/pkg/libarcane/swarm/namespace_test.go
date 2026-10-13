// Source: getarcaneapp/arcane/backend/pkg/libarcane/swarm/namespace_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package swarm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNamespaceResolve(t *testing.T) {
	ns := namespace{name: "stack"}

	require.Equal(t, "stack_data", ns.Scope("data"))
	require.Equal(t, "stack_data", ns.Resolve("data", ""))
	require.Equal(t, "custom", ns.Resolve("data", " custom "))
}

func TestValidateStackNameInternal(t *testing.T) {
	require.NoError(t, validateStackNameInternal("stack-1"))
	require.Error(t, validateStackNameInternal("Stack-1"))
	require.Error(t, validateStackNameInternal("../stack"))
}
