// Source: docklet-next/docs/design/network-management.md
// Revision: 09d1e632cb3a14bd48b87232acaf991b9d12145e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original host-side network contract implementation for issue #203.

package network

import (
	"net/http"
	"net/netip"
	"regexp"
	"strings"

	networktypes "github.com/florianfinn/docklet-next/types/network"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker"
)

var networkName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)
var parentName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,14}$`)
var aliasName = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

func rejection(status int, code, detail string) *networktypes.Error {
	return &networktypes.Error{Status: status, Title: http.StatusText(status), Detail: detail, Code: "network." + code}
}
func invalidNetwork(detail string) error { return rejection(400, "invalid-network", detail) }

// Existing networks must come directly from this host's daemon, without the
// public inventory filter: hidden system networks also reserve names and pools.
func validateCreate(name string, opts client.NetworkCreateOptions, existing []network.Summary) error {
	if !networkName.MatchString(name) || docker.IsDefaultNetwork(strings.ToLower(name)) || strings.EqualFold(name, "default") {
		return invalidNetwork("Invalid or reserved network name")
	}
	for _, n := range existing {
		if n.Name == name {
			return rejection(409, "exists", "Network name already exists")
		}
	}
	driver := opts.Driver
	if driver == "" {
		driver = "bridge"
	}
	if driver != "bridge" && driver != "macvlan" && driver != "ipvlan" {
		return invalidNetwork("Unsupported network driver")
	}
	if driver != "bridge" && !parentName.MatchString(opts.Options["parent"]) {
		return invalidNetwork("A valid parent interface is required")
	}
	if opts.Internal && driver != "bridge" {
		return invalidNetwork("Internal networks require the bridge driver")
	}
	v4, v6 := opts.EnableIPv4 == nil || *opts.EnableIPv4, opts.EnableIPv6 != nil && *opts.EnableIPv6
	if !v4 && !v6 {
		return invalidNetwork("At least one address family is required")
	}
	for _, entries := range []map[string]string{opts.Labels, opts.Options} {
		for key := range entries {
			if strings.TrimSpace(key) == "" {
				return invalidNetwork("Label and option keys must be nonempty")
			}
		}
	}
	seen := map[bool]bool{}
	if opts.IPAM != nil {
		if (opts.IPAM.Driver != "" && opts.IPAM.Driver != "default") || len(opts.IPAM.Options) != 0 {
			return invalidNetwork("Custom IPAM is unsupported")
		}
		for _, pool := range opts.IPAM.Config {
			prefix := pool.Subnet
			if !prefix.IsValid() || prefix.Addr().Is4In6() || prefix != prefix.Masked() {
				return invalidNetwork("Subnet must be a canonical CIDR")
			}
			is4 := prefix.Addr().Is4()
			if seen[is4] || (is4 && !v4) || (!is4 && !v6) {
				return invalidNetwork("Only one pool per enabled family is allowed")
			}
			seen[is4] = true
			if pool.Gateway.IsValid() && !usableAddress(prefix, pool.Gateway) {
				return invalidNetwork("Gateway must be usable inside its subnet")
			}
			if pool.IPRange.IsValid() && (pool.IPRange != pool.IPRange.Masked() || !prefix.Contains(pool.IPRange.Addr()) || pool.IPRange.Bits() < prefix.Bits() || pool.IPRange.Addr().BitLen() != prefix.Addr().BitLen()) {
				return invalidNetwork("IP range must be inside its subnet")
			}
			for key, addr := range pool.AuxAddress {
				if strings.TrimSpace(key) == "" || !prefix.Contains(addr) || addr == pool.Gateway {
					return invalidNetwork("Invalid auxiliary address reservation")
				}
			}
			if driver != "bridge" && !pool.Gateway.IsValid() {
				return invalidNetwork("LAN networks require a gateway for each pool")
			}
			for _, n := range existing {
				for _, other := range n.IPAM.Config {
					if prefix.Overlaps(other.Subnet) {
						return rejection(409, "overlap", "Subnet overlaps an existing network on this host")
					}
				}
			}
		}
	}
	if driver != "bridge" && ((v4 && !seen[true]) || (v6 && !seen[false])) {
		return invalidNetwork("LAN networks require a subnet for every enabled family")
	}
	return nil
}

func usableAddress(prefix netip.Prefix, addr netip.Addr) bool {
	if !prefix.Contains(addr) || addr == prefix.Masked().Addr() {
		return false
	}
	if addr.Is4() {
		bytes := addr.As4()
		n := uint32(bytes[0])<<24 | uint32(bytes[1])<<16 | uint32(bytes[2])<<8 | uint32(bytes[3])
		mask := ^uint32(0) >> prefix.Bits()
		if n&mask == mask {
			return false
		}
	}
	return true
}

func attached(n network.Inspect, networks map[string]*network.EndpointSettings) bool {
	for name, endpoint := range networks {
		if name == n.Name || (endpoint != nil && endpoint.NetworkID == n.ID) {
			return true
		}
	}
	return false
}

func validateConnect(n network.Inspect, c container.InspectResponse, all []container.InspectResponse, req networktypes.ConnectContainerRequest) (*network.EndpointSettings, error) {
	if n.Name == "host" || n.Name == "none" || n.Driver == "host" || n.Driver == "null" {
		return nil, rejection(409, "builtin-network", "Network does not allow container connections")
	}
	if c.HostConfig != nil {
		mode := string(c.HostConfig.NetworkMode)
		if mode == "host" || mode == "none" || strings.HasPrefix(mode, "container:") {
			return nil, rejection(409, "network-mode", "Container network mode does not allow connections")
		}
	}
	if c.NetworkSettings != nil && attached(n, c.NetworkSettings.Networks) {
		return nil, rejection(409, "already-connected", "Container is already connected")
	}
	endpoint := &network.EndpointSettings{}
	seen := map[string]bool{}
	for _, alias := range req.Aliases {
		if !aliasName.MatchString(alias) {
			return nil, invalidNetwork("Aliases must be nonempty DNS labels")
		}
		key := strings.ToLower(alias)
		if !seen[key] {
			endpoint.Aliases = append(endpoint.Aliases, alias)
			seen[key] = true
		}
	}
	ipam := &network.EndpointIPAMConfig{}
	for _, field := range []struct {
		text   string
		v4     bool
		target *netip.Addr
	}{{req.IPv4Address, true, &ipam.IPv4Address}, {req.IPv6Address, false, &ipam.IPv6Address}} {
		if strings.TrimSpace(field.text) == "" {
			continue
		}
		addr, err := netip.ParseAddr(strings.TrimSpace(field.text))
		if err != nil || addr.Is4() != field.v4 || addr.Is4In6() || addr.Zone() != "" {
			return nil, rejection(400, "invalid-ip", "Fixed address has the wrong family or syntax")
		}
		if err := validateFixedAddress(n, all, addr); err != nil {
			return nil, err
		}
		*field.target = addr
	}
	if ipam.IPv4Address.IsValid() || ipam.IPv6Address.IsValid() {
		endpoint.IPAMConfig = ipam
	}
	return endpoint, nil
}

func validateFixedAddress(n network.Inspect, all []container.InspectResponse, addr netip.Addr) error {
	valid := false
	for _, pool := range n.IPAM.Config {
		if pool.Subnet.IsValid() && pool.Subnet.Addr().Is4() == addr.Is4() {
			if !usableAddress(pool.Subnet, addr) || addr == pool.Gateway {
				return rejection(400, "invalid-ip", "Fixed address must be usable inside the configured subnet")
			}
			for _, reserved := range pool.AuxAddress {
				if addr == reserved {
					return rejection(400, "invalid-ip", "Fixed address is reserved")
				}
			}
			valid = true
		}
	}
	if !valid {
		return rejection(400, "invalid-ip", "Fixed addresses require a configured subnet of the same family")
	}
	for _, endpoint := range n.Containers {
		if (endpoint.IPv4Address.IsValid() && endpoint.IPv4Address.Addr() == addr) || (endpoint.IPv6Address.IsValid() && endpoint.IPv6Address.Addr() == addr) {
			return rejection(400, "invalid-ip", "Fixed address is already taken")
		}
	}
	// Stopped endpoints retain requested addresses in IPAMConfig while their
	// operational addresses and NetworkInspect.Containers can be empty.
	for _, c := range all {
		if c.NetworkSettings == nil {
			continue
		}
		for name, endpoint := range c.NetworkSettings.Networks {
			if endpoint == nil || (name != n.Name && endpoint.NetworkID != n.ID) {
				continue
			}
			if endpoint.IPAddress == addr || endpoint.GlobalIPv6Address == addr || (endpoint.IPAMConfig != nil && (endpoint.IPAMConfig.IPv4Address == addr || endpoint.IPAMConfig.IPv6Address == addr)) {
				return rejection(400, "invalid-ip", "Fixed address is already taken")
			}
		}
	}
	return nil
}

func validateDisconnect(n network.Inspect, c container.InspectResponse) error {
	if n.Name == "host" || n.Name == "none" || n.Driver == "host" || n.Driver == "null" {
		return rejection(409, "network-mode", "Network mode cannot be disconnected")
	}
	if c.NetworkSettings == nil || !attached(n, c.NetworkSettings.Networks) {
		return rejection(409, "not-connected", "Container is not connected")
	}
	return nil
}
