// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original graceful defaults for absent parsed configuration fields.

package traefik

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOptionalNodeReadersKeepEmptyDefaults(t *testing.T) {
	d, err := parseDocument("field: value\n", "")
	require.NoError(t, err)
	absent := field(d.root, "absent")
	require.NotPanics(t, func() { require.Equal(t, "", str(absent)) })
	require.NotPanics(t, func() { require.Empty(t, list(absent)) })
	require.NotPanics(t, func() { require.Empty(t, keys(absent)) })
	require.NotPanics(t, func() { require.Nil(t, field(absent, "nested")) })
	require.NotPanics(t, func() { require.Nil(t, value(absent)) })
}
