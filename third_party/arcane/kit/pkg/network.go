// Source: getarcaneapp/kit/pkg/network.go
// Revision: 9c61043fc52d1977d644f075d9c168335c3a6df7
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package kit

import (
	"cmp"
	"net/netip"
	"strings"
)

// CompareAddresses orders IP address strings numerically, ignoring any CIDR suffix.
// Empty values sort first and malformed values last in string order.
func CompareAddresses(a, b string) int {
	if a == "" || b == "" {
		return strings.Compare(a, b)
	}
	hostA, _, _ := strings.Cut(a, "/")
	hostB, _, _ := strings.Cut(b, "/")
	addrA, errA := netip.ParseAddr(hostA)
	addrB, errB := netip.ParseAddr(hostB)
	switch {
	case errA == nil && errB == nil:
		return addrA.Compare(addrB)
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

// CompareSubnets orders CIDR strings by masked network address, then prefix length.
// Empty values sort first and malformed values last in string order.
func CompareSubnets(a, b string) int {
	if a == "" || b == "" {
		return strings.Compare(a, b)
	}
	prefixA, errA := netip.ParsePrefix(a)
	prefixB, errB := netip.ParsePrefix(b)
	switch {
	case errA == nil && errB == nil:
		prefixA, prefixB = prefixA.Masked(), prefixB.Masked()
		return cmp.Or(prefixA.Addr().Compare(prefixB.Addr()), cmp.Compare(prefixA.Bits(), prefixB.Bits()))
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}
