// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Shared containerd platform normalization and comparison boundary.

package platformmatch

import (
	"github.com/containerd/platforms"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Normalize uses containerd's canonical OS, architecture and variant rules.
// Docker info can also use uname's ARM aliases, which are not OCI names.
func Normalize(p ocispec.Platform) ocispec.Platform {
	switch p.Architecture {
	case "armv7l":
		p.Architecture, p.Variant = "arm", "v7"
	case "armv6l":
		p.Architecture, p.Variant = "arm", "v6"
	}
	return platforms.Normalize(p)
}

// Equal admits canonical aliases, but never another runnable sub-platform:
// native image identity requires the exact platform, not CPU compatibility.
func Equal(a, b ocispec.Platform) bool {
	if a.OS == "" || a.Architecture == "" || b.OS == "" || b.Architecture == "" {
		return false
	}
	return platforms.OnlyStrict(Normalize(a)).Match(Normalize(b))
}

// Compatible preserves the ordered CPU fallback used by attestation discovery.
func Compatible(wanted, candidate ocispec.Platform) bool {
	return platforms.Only(Normalize(wanted)).Match(Normalize(candidate))
}
