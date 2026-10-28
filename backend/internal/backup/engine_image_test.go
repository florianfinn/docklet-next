// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original engine reference selection and release-default proof.

package backup

import (
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEngineImageUsesHostConfigurationWithoutChangingTools(t *testing.T) {
	e := NewEngine(t.Context(), nil, nil)
	require.Equal(t, rustic.DefaultImage, e.EngineImage(t.Context()))
	reference := "registry.example.com/rustic:0.11.2"
	e.SetImageReference(func() string { return reference })
	require.Equal(t, reference, e.EngineImage(t.Context()))
	reference = ""
	require.Equal(t, rustic.DefaultImage, e.EngineImage(t.Context()))
}
