// Source: docklet-next/docs/design/network-management.md
// Revision: 09d1e632cb3a14bd48b87232acaf991b9d12145e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original host-side network contract implementation for issue #203.

package network

import (
	"net/netip"
	"testing"

	networktypes "github.com/florianfinn/docklet-next/types/network"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

func pool4() network.IPAMConfig {
	return network.IPAMConfig{Subnet: netip.MustParsePrefix("192.0.2.0/24"), Gateway: netip.MustParseAddr("192.0.2.1")}
}
func pool6() network.IPAMConfig {
	return network.IPAMConfig{Subnet: netip.MustParsePrefix("2001:db8::/64"), Gateway: netip.MustParseAddr("2001:db8::1")}
}
func options4() client.NetworkCreateOptions {
	return client.NetworkCreateOptions{Driver: "bridge", IPAM: &network.IPAM{Config: []network.IPAMConfig{pool4()}}}
}
func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	if code == "" {
		require.NoError(t, err)
		return
	}
	var problem *networktypes.Error
	require.ErrorAs(t, err, &problem)
	require.Equal(t, "network."+code, problem.Code)
}

func TestCreateValidation(t *testing.T) {
	cases := []struct {
		name   string
		change func(*client.NetworkCreateOptions)
		code   string
	}{
		{"IPv4 bridge", func(*client.NetworkCreateOptions) {}, ""},
		{"daemon assigned bridge", func(o *client.NetworkCreateOptions) { o.Driver = ""; o.IPAM = nil }, ""},
		{"dual stack", func(o *client.NetworkCreateOptions) {
			o.EnableIPv6 = new(true)
			o.IPAM.Config = append(o.IPAM.Config, pool6())
		}, ""},
		{"IPv6 only", func(o *client.NetworkCreateOptions) {
			o.EnableIPv4 = new(false)
			o.EnableIPv6 = new(true)
			o.IPAM.Config = []network.IPAMConfig{pool6()}
		}, ""},
		{"macvlan", func(o *client.NetworkCreateOptions) {
			o.Driver = "macvlan"
			o.Options = map[string]string{"parent": "eth0"}
		}, ""},
		{"macvlan missing parent", func(o *client.NetworkCreateOptions) { o.Driver = "macvlan" }, "invalid-network"},
		{"invalid parent", func(o *client.NetworkCreateOptions) {
			o.Driver = "macvlan"
			o.Options = map[string]string{"parent": "bad/parent"}
		}, "invalid-network"},
		{"ipvlan", func(o *client.NetworkCreateOptions) {
			o.Driver = "ipvlan"
			o.Options = map[string]string{"parent": "eth0.2"}
		}, ""},
		{"overlay", func(o *client.NetworkCreateOptions) { o.Driver = "overlay" }, "invalid-network"},
		{"gateway outside", func(o *client.NetworkCreateOptions) { o.IPAM.Config[0].Gateway = netip.MustParseAddr("198.51.100.1") }, "invalid-network"},
		{"gateway network", func(o *client.NetworkCreateOptions) { o.IPAM.Config[0].Gateway = netip.MustParseAddr("192.0.2.0") }, "invalid-network"},
		{"gateway broadcast", func(o *client.NetworkCreateOptions) { o.IPAM.Config[0].Gateway = netip.MustParseAddr("192.0.2.255") }, "invalid-network"},
		{"gateway wrong family", func(o *client.NetworkCreateOptions) { o.IPAM.Config[0].Gateway = netip.MustParseAddr("2001:db8::1") }, "invalid-network"},
		{"range outside", func(o *client.NetworkCreateOptions) {
			o.IPAM.Config[0].IPRange = netip.MustParsePrefix("198.51.100.0/25")
		}, "invalid-network"},
		{"range enclosing subnet", func(o *client.NetworkCreateOptions) { o.IPAM.Config[0].IPRange = netip.MustParsePrefix("192.0.2.0/23") }, "invalid-network"},
		{"range inside", func(o *client.NetworkCreateOptions) {
			o.IPAM.Config[0].IPRange = netip.MustParsePrefix("192.0.2.128/25")
		}, ""},
		{"aux outside", func(o *client.NetworkCreateOptions) {
			o.IPAM.Config[0].AuxAddress = map[string]netip.Addr{"reserved": netip.MustParseAddr("198.51.100.2")}
		}, "invalid-network"},
		{"aux gateway", func(o *client.NetworkCreateOptions) {
			o.IPAM.Config[0].AuxAddress = map[string]netip.Addr{"reserved": netip.MustParseAddr("192.0.2.1")}
		}, "invalid-network"},
		{"empty aux name", func(o *client.NetworkCreateOptions) {
			o.IPAM.Config[0].AuxAddress = map[string]netip.Addr{" ": netip.MustParseAddr("192.0.2.2")}
		}, "invalid-network"},
		{"noncanonical subnet", func(o *client.NetworkCreateOptions) { o.IPAM.Config[0].Subnet = netip.MustParsePrefix("192.0.2.7/24") }, "invalid-network"},
		{"IPv4 pool while only IPv6 enabled", func(o *client.NetworkCreateOptions) { o.EnableIPv4 = new(false); o.EnableIPv6 = new(true) }, "invalid-network"},
		{"IPv6 pool while disabled", func(o *client.NetworkCreateOptions) { o.IPAM.Config = []network.IPAMConfig{pool6()} }, "invalid-network"},
		{"duplicate pool family", func(o *client.NetworkCreateOptions) { o.IPAM.Config = append(o.IPAM.Config, pool4()) }, "invalid-network"},
		{"internal LAN", func(o *client.NetworkCreateOptions) {
			o.Driver = "macvlan"
			o.Internal = true
			o.Options = map[string]string{"parent": "eth0"}
		}, "invalid-network"},
		{"no family", func(o *client.NetworkCreateOptions) { o.EnableIPv4 = new(false) }, "invalid-network"},
		{"empty label", func(o *client.NetworkCreateOptions) { o.Labels = map[string]string{" ": "value"} }, "invalid-network"},
		{"empty option", func(o *client.NetworkCreateOptions) { o.Options = map[string]string{"": "value"} }, "invalid-network"},
		{"custom IPAM", func(o *client.NetworkCreateOptions) { o.IPAM.Driver = "custom" }, "invalid-network"},
		{"LAN missing pool", func(o *client.NetworkCreateOptions) {
			o.Driver = "ipvlan"
			o.Options = map[string]string{"parent": "eth0"}
			o.IPAM = nil
		}, "invalid-network"},
		{"LAN missing gateway", func(o *client.NetworkCreateOptions) {
			o.Driver = "ipvlan"
			o.Options = map[string]string{"parent": "eth0"}
			o.IPAM.Config[0].Gateway = netip.Addr{}
		}, "invalid-network"},
		{"IPv6 last address is usable", func(o *client.NetworkCreateOptions) {
			o.EnableIPv4 = new(false)
			o.EnableIPv6 = new(true)
			p := pool6()
			p.Gateway = netip.MustParseAddr("2001:db8::ffff:ffff:ffff:ffff")
			o.IPAM.Config = []network.IPAMConfig{p}
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := options4()
			tc.change(&o)
			requireCode(t, validateCreate("example", o, nil), tc.code)
		})
	}
	for _, name := range []string{"", "bridge", "HOST", "none", "default", "ingress", "bad/name", "-name"} {
		t.Run("name="+name, func(t *testing.T) { requireCode(t, validateCreate(name, options4(), nil), "invalid-network") })
	}
	t.Run("duplicate name", func(t *testing.T) {
		requireCode(t, validateCreate("example", options4(), []network.Summary{{Name: "example"}}), "exists")
	})
	for _, v6 := range []bool{false, true} {
		t.Run(map[bool]string{false: "overlap IPv4", true: "overlap IPv6"}[v6], func(t *testing.T) {
			o := options4()
			if v6 {
				o.EnableIPv6 = new(true)
				o.IPAM.Config = []network.IPAMConfig{pool6()}
			}
			existing := []network.Summary{{Name: "other", IPAM: *o.IPAM}}
			requireCode(t, validateCreate("example", o, existing), "overlap")
			// A host-scoped snapshot never includes another daemon's networks.
			require.NoError(t, validateCreate("example", o, nil))
		})
	}
}

func inspectNetwork() network.Inspect {
	return network.Inspect{Network: network.Network{ID: "net1", Name: "example", Driver: "bridge", EnableIPv4: true, EnableIPv6: true, IPAM: network.IPAM{Config: []network.IPAMConfig{pool4(), pool6()}}}}
}
func disconnectedContainer() container.InspectResponse {
	return container.InspectResponse{ID: "container1", HostConfig: &container.HostConfig{NetworkMode: "bridge"}, NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}}}
}
func stoppedReference() container.InspectResponse {
	c := disconnectedContainer()
	c.ID = "stopped"
	c.State = &container.State{Running: false}
	c.NetworkSettings.Networks["example"] = &network.EndpointSettings{NetworkID: "net1", IPAMConfig: &network.EndpointIPAMConfig{IPv4Address: netip.MustParseAddr("192.0.2.7"), IPv6Address: netip.MustParseAddr("2001:db8::7")}}
	return c
}

func TestConnectValidation(t *testing.T) {
	cases := []struct{ name, v4, v6, code string }{
		{"fixed IPv4", "192.0.2.7", "", ""},
		{"fixed IPv6", "", "2001:db8::7", ""},
		{"outside subnet", "198.51.100.7", "", "invalid-ip"},
		{"gateway", "192.0.2.1", "", "invalid-ip"},
		{"network address", "192.0.2.0", "", "invalid-ip"},
		{"broadcast", "192.0.2.255", "", "invalid-ip"},
		{"IPv6 last address", "", "2001:db8::ffff:ffff:ffff:ffff", ""},
		{"IPv4 in IPv6 field", "", "192.0.2.7", "invalid-ip"},
		{"IPv6 in IPv4 field", "2001:db8::7", "", "invalid-ip"},
		{"bad address", "bad", "", "invalid-ip"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateConnect(inspectNetwork(), disconnectedContainer(), nil, networktypes.ConnectContainerRequest{IPv4Address: tc.v4, IPv6Address: tc.v6})
			requireCode(t, err, tc.code)
		})
	}
	t.Run("stopped container fixed IPv6", func(t *testing.T) {
		c := disconnectedContainer()
		c.State = &container.State{Running: false}
		_, err := validateConnect(inspectNetwork(), c, nil, networktypes.ConnectContainerRequest{IPv6Address: "2001:db8::7"})
		require.NoError(t, err)
	})
	for _, field := range []string{"IPv4", "IPv6"} {
		t.Run("address taken by stopped container "+field, func(t *testing.T) {
			req := networktypes.ConnectContainerRequest{}
			if field == "IPv4" {
				req.IPv4Address = "192.0.2.7"
			} else {
				req.IPv6Address = "2001:db8::7"
			}
			_, err := validateConnect(inspectNetwork(), disconnectedContainer(), []container.InspectResponse{stoppedReference()}, req)
			requireCode(t, err, "invalid-ip")
		})
	}
	t.Run("already connected stopped", func(t *testing.T) {
		_, err := validateConnect(inspectNetwork(), stoppedReference(), nil, networktypes.ConnectContainerRequest{})
		requireCode(t, err, "already-connected")
	})
	t.Run("without IPAM", func(t *testing.T) {
		n := inspectNetwork()
		n.IPAM.Config = nil
		_, err := validateConnect(n, disconnectedContainer(), nil, networktypes.ConnectContainerRequest{IPv4Address: "192.0.2.7"})
		requireCode(t, err, "invalid-ip")
	})
	t.Run("aux reserved", func(t *testing.T) {
		n := inspectNetwork()
		n.IPAM.Config[0].AuxAddress = map[string]netip.Addr{"reserved": netip.MustParseAddr("192.0.2.7")}
		_, err := validateConnect(n, disconnectedContainer(), nil, networktypes.ConnectContainerRequest{IPv4Address: "192.0.2.7"})
		requireCode(t, err, "invalid-ip")
	})
	for _, mode := range []string{"host", "none", "container:other"} {
		t.Run(mode, func(t *testing.T) {
			c := disconnectedContainer()
			c.HostConfig.NetworkMode = container.NetworkMode(mode)
			_, err := validateConnect(inspectNetwork(), c, nil, networktypes.ConnectContainerRequest{})
			requireCode(t, err, "network-mode")
		})
	}
	for _, name := range []string{"host", "none"} {
		t.Run("network "+name, func(t *testing.T) {
			n := inspectNetwork()
			n.Name = name
			_, err := validateConnect(n, disconnectedContainer(), nil, networktypes.ConnectContainerRequest{})
			requireCode(t, err, "builtin-network")
		})
	}
	t.Run("alias deduplication", func(t *testing.T) {
		ep, err := validateConnect(inspectNetwork(), disconnectedContainer(), nil, networktypes.ConnectContainerRequest{Aliases: []string{"example", "EXAMPLE", "api-2"}})
		require.NoError(t, err)
		require.Equal(t, []string{"example", "api-2"}, ep.Aliases)
	})
	for _, alias := range []string{"", "bad.alias", "-bad", "bad-"} {
		t.Run("alias "+alias, func(t *testing.T) {
			_, err := validateConnect(inspectNetwork(), disconnectedContainer(), nil, networktypes.ConnectContainerRequest{Aliases: []string{alias}})
			requireCode(t, err, "invalid-network")
		})
	}
}

func TestDisconnectValidation(t *testing.T) {
	require.NoError(t, validateDisconnect(inspectNetwork(), stoppedReference()))
	requireCode(t, validateDisconnect(inspectNetwork(), disconnectedContainer()), "not-connected")
}
