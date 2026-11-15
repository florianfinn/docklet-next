// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original owned observed execution context values.

package updater

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestObservedExecutionOwnsSuppressedCandidates(t *testing.T) {
	value := ObservedExecution{Mode: "scheduled", PolicyID: "policy", ChildID: "child", Suppressed: []SuppressedCandidate{{Repository: "registry.example.com/app"}}}
	ctx := WithObservedExecution(t.Context(), value)
	value.Suppressed[0].Repository = "registry.example.com/mutated"
	got := ObservedExecutionFromContext(ctx)
	require.Equal(t, "registry.example.com/app", got.Suppressed[0].Repository)
	got.Suppressed[0].Repository = "registry.example.com/mutated-again"
	require.Equal(t, "registry.example.com/app", ObservedExecutionFromContext(ctx).Suppressed[0].Repository)
	require.Empty(t, ObservedExecutionFromContext(t.Context()).Mode)
}
