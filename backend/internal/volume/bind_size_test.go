// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package volume

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBindSizeAggregation(t *testing.T) {
	sum := bindSizeSum{device: "8", seen: map[string]bool{}}
	for _, line := range []string{"8 12 2 10 regular file", "8 12 2 10 regular file", "8 13 1 4 regular file", "9 14 1 100 regular file", "8 15 1 4096 directory", "bad row", "8 16 1 -1 regular file"} {
		sum.add(line)
	}
	require.EqualValues(t, 14, sum.size)
	require.True(t, sum.partial)
}
