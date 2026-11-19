// Source: docklet-next/issues/529
// Revision: 46c3a29f25da7a67c795e7bf082996b8db0e4b96
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original link base resolver regression coverage for local hub addresses.

package notification

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveLinkBaseLocalAddresses(t *testing.T) {
	for _, tc := range []struct{ linkBase, domain, address, want string }{
		{"auto", "https://hub.example.com", "192.0.2.10:3552", "https://hub.example.com"},
		{"internal", "https://hub.example.com", "192.0.2.10:3552", "http://192.0.2.10:3552"},
		{"internal", "", "2001:db8::10", "http://[2001:db8::10]"},
		{"internal", "", "[2001:db8::10]:3552", "http://[2001:db8::10]:3552"},
		{"internal", "", "::ffff:192.0.2.10", "http://[::ffff:192.0.2.10]"},
		{"internal", "", "::ffff:c000:20a", "http://[::ffff:c000:20a]"},
	} {
		got := resolveLinkBase(tc.linkBase, tc.domain, tc.address, "https://app.example.com")
		require.Equal(t, tc.want, got)
		parsed, err := url.Parse(got)
		require.NoError(t, err)
		require.NotEmpty(t, parsed.Hostname())
	}
}
