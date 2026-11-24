// Source: docklet-next/issues/601
// Revision: 22319d6f16ef401b1c247c7a4eae599676564095
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original live-snapshot host address projection proofs.

package tailnet

import (
	dto "github.com/florianfinn/docklet-next/types/tailnet"
	"github.com/stretchr/testify/require"
	"net/netip"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
	"testing"
)

func TestHostPeerProjectsCurrentNameAndAddresses(t *testing.T) {
	m := NewManager(ManagerConfig{})
	m.conn = dto.Connection{State: "connected"}
	peer := &ipnstate.PeerStatus{ID: "example-node", DNSName: "agent.example.com.", TailscaleIPs: []netip.Addr{netip.MustParseAddr("2001:db8::42"), netip.MustParseAddr("192.0.2.42")}}
	m.status = &ipnstate.Status{Peer: map[key.NodePublic]*ipnstate.PeerStatus{key.NewNode().Public(): peer}}
	out := m.HostPeer("example-node", nil, nil)
	require.Equal(t, "agent.example.com", out.DNSName)
	require.Equal(t, []string{"192.0.2.42", "2001:db8::42"}, out.Addresses)
	peer.DNSName = "renamed.example.com."
	peer.TailscaleIPs = []netip.Addr{netip.MustParseAddr("192.0.2.43")}
	out = m.HostPeer("example-node", nil, nil)
	require.Equal(t, "renamed.example.com", out.DNSName)
	require.Equal(t, []string{"192.0.2.43"}, out.Addresses)
	require.Empty(t, m.HostPeer("missing", nil, nil).Addresses)
	m.conn.State = "off"
	require.Empty(t, m.HostPeer("example-node", nil, nil).DNSName)
}
