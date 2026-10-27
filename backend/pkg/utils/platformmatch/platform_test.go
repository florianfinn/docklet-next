// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Review correction with real producer and behavioral guard proofs.

package platformmatch

import (
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPlatformCanonicalMatching(t *testing.T) {
	for _, arch := range []string{"arm", "arm64"} {
		variant := map[string]string{"arm": "v7", "arm64": "v8"}[arch]
		plain := ocispec.Platform{OS: "linux", Architecture: arch}
		explicit := plain
		explicit.Variant = variant
		require.True(t, Equal(plain, explicit))
		require.True(t, Equal(explicit, plain))
		require.Equal(t, Normalize(plain), Normalize(explicit))
	}
	require.False(t, Equal(ocispec.Platform{OS: "linux", Architecture: "arm", Variant: "v7"}, ocispec.Platform{OS: "linux", Architecture: "arm", Variant: "v6"}))
	require.False(t, Equal(ocispec.Platform{}, ocispec.Platform{}))
	require.True(t, Compatible(ocispec.Platform{OS: "linux", Architecture: "arm", Variant: "v7"}, ocispec.Platform{OS: "linux", Architecture: "arm", Variant: "v6"}))
}
