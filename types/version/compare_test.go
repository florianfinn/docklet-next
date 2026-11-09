// Source: docklet-next/issues/235
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original product version precedence tests.

package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseProductRejectsNonSemVer(t *testing.T) {
	for _, raw := range []string{"", "dev", "1.0", "v1", "1.0.0.1000", "01.0.0", "1.00.0", "1.0.0-", "1.0.0-rc..1", "1.0.0-rc.01",
		"1.0.0-rc_1", "1.0.0+", "1.0.0+a..b", "latest", "v1.0.0 extra", "-1.0.0"} {
		_, ok := ParseProduct(raw)
		require.False(t, ok, raw)
	}
	for _, raw := range []string{"1.0.0", "v1.0.0", "1.0.0-rc.1", "1.0.0-0.3.7", "1.0.0-x-y.7", "1.0.0+build.007", "0.0.0"} {
		_, ok := ParseProduct(raw)
		require.True(t, ok, raw)
	}
}

func TestCompareProductFollowsSemVerPrecedence(t *testing.T) {
	// SemVer 2 section 11 example chain, plus rc ordering used by release candidates.
	chain := []string{"1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11",
		"1.0.0-rc.1", "1.0.0-rc.2", "1.0.0-rc.10", "1.0.0", "1.0.1", "1.1.0", "2.0.0"}
	for i := range chain {
		for j := range chain {
			got, ok := CompareProduct(chain[i], chain[j])
			require.True(t, ok)
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			require.Equal(t, want, got, "%s vs %s", chain[i], chain[j])
		}
	}
	got, ok := CompareProduct("v1.0.0+build.1", "1.0.0")
	require.True(t, ok)
	require.Zero(t, got)
	_, ok = CompareProduct("dev", "1.0.0")
	require.False(t, ok)
}
