// Source: docklet-next/issues/519
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original pure container-aware candidate builder and trusted request host parsing.

package environment

import (
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	environmenttypes "github.com/florianfinn/docklet-next/types/environment"
	"github.com/moby/moby/api/types/network"
)

// discoveryRequestHost mirrors bootstrap's TRUSTED_PROXIES CIDR/direct-peer policy.
// AUTH_TRUSTED_PROXIES is a separate identity admission policy, not forwarding trust.
// A trusted X-Forwarded-Proto of https is kept as an "https://" prefix for the request candidate.
func discoveryRequestHost(trusted, host, forwarded, forwardedProto, peer string) string {
	remote, _, err := net.SplitHostPort(peer)
	if err != nil {
		remote = peer
	}
	ip := net.ParseIP(remote)
	for entry := range strings.SplitSeq(trusted, ",") {
		_, prefix, err := net.ParseCIDR(strings.TrimSpace(entry))
		if err == nil && ip != nil && prefix.Contains(ip) {
			if forwarded != "" {
				host = forwarded
			}
			if strings.EqualFold(strings.TrimSpace(forwardedProto), "https") && validHubHost(host) {
				return "https://" + host
			}
			break
		}
	}
	if !validHubHost(host) {
		return ""
	}
	return host
}

func validHubHost(host string) bool {
	if host == "" || strings.ContainsAny(host, " /\\\t\r\n,@?#%") {
		return false
	}
	parsed, err := url.Parse("http://" + host)
	if err != nil || parsed.Host != host || parsed.Hostname() == "" || parsed.User != nil {
		return false
	}
	name := parsed.Hostname()
	if strings.Contains(host, ":") {
		if net.ParseIP(name) != nil && host == "["+name+"]" {
			return true
		}
		if _, port, err := net.SplitHostPort(host); err != nil || !validHubPort(port) {
			return false
		}
	}
	if net.ParseIP(name) != nil {
		return true
	}
	if len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return true
}

func validHubPort(port string) bool {
	n, err := strconv.Atoi(port)
	return err == nil && n > 0 && n <= 65535
}

func hubListenPort(cfg *config.Config) string {
	if cfg.Port == "" {
		return "3552"
	}
	return cfg.Port
}

func usableHubInterface(raw string) bool {
	ip := net.ParseIP(raw)
	return ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast() && !ip.IsMulticast()
}

func buildHubDiscovery(cfg *config.Config, publicURL, hubAddress string, self *hubSelf, addresses []string, requestHost string) (environmenttypes.HubDiscovery, error) {
	image, err := agentImageReference(cfg)
	if err != nil {
		return environmenttypes.HubDiscovery{}, err
	}
	result := environmenttypes.HubDiscovery{DetectedAt: time.Now().UTC(), AgentImage: image, ContainerDetection: "unknown", Candidates: []environmenttypes.HubAddressCandidate{}}
	seen := map[string]bool{}
	add := func(raw, source string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if !strings.Contains(raw, "://") {
			raw = "http://" + raw
		}
		parsed, err := url.Parse(raw)
		// Configured values keep the earlier acceptance (container names, IDN); only the request host is strict.
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return
		}
		parsed.Path = ""
		value := parsed.String()
		if seen[value] {
			return
		}
		seen[value] = true
		kind, reach := "domain", "unknown"
		if ip := net.ParseIP(parsed.Hostname()); ip != nil {
			kind = "ip"
			if ip.IsLoopback() {
				reach = "same-host"
			}
		} else if strings.EqualFold(parsed.Hostname(), "localhost") {
			reach = "same-host"
		}
		result.Candidates = append(result.Candidates, environmenttypes.HubAddressCandidate{ID: source + ":" + value, URL: value, Reach: reach, Source: source, Kind: kind})
	}
	scheme := "http://"
	if cfg.TLSEnabled {
		scheme = "https://"
	}
	addIP := func(address, port, source string) { add(scheme+net.JoinHostPort(address, port), source) }
	add(publicURL, "configured")
	add(cfg.GetAppURL(), "configured")
	add(hubAddress, "configured")
	requestIP, requestScheme := "", scheme
	if rest, ok := strings.CutPrefix(requestHost, "https://"); ok {
		requestHost, requestScheme = rest, "https://"
	}
	if validHubHost(requestHost) {
		parsed, _ := url.Parse("http://" + requestHost)
		if ip := net.ParseIP(parsed.Hostname()); ip != nil {
			requestIP = ip.String()
		}
	} else {
		requestHost = ""
	}
	requestBindings := []environmenttypes.HubPublishedPort{}
	if self != nil {
		result.ContainerDetection = "container"
		listenPort, _ := strconv.Atoi(hubListenPort(cfg))
		result.Container = &environmenttypes.HubContainer{Name: strings.TrimPrefix(self.inspect.Name, "/"), ListenPort: listenPort, Networks: []environmenttypes.HubContainerNetwork{}, PublishedPorts: []environmenttypes.HubPublishedPort{}}
		hostNetwork := self.inspect.HostConfig != nil && self.inspect.HostConfig.NetworkMode == "host"
		interfaceIPs := []string{}
		for _, name := range sortedHubNetworkNames(self.inspect) {
			driver := self.drivers[name]
			result.Container.Networks = append(result.Container.Networks, environmenttypes.HubContainerNetwork{Name: name, Driver: driver})
			if driver == "host" {
				hostNetwork = true
			}
			if driver == "macvlan" || driver == "ipvlan" {
				if endpoint := self.inspect.NetworkSettings.Networks[name]; endpoint != nil {
					interfaceIPs = append(interfaceIPs, endpoint.IPAddress.String(), endpoint.GlobalIPv6Address.String())
				}
			}
		}
		if hostNetwork {
			interfaceIPs = append(interfaceIPs, addresses...)
		}
		if hostNetwork && len(result.Container.Networks) == 0 {
			result.Container.Networks = append(result.Container.Networks, environmenttypes.HubContainerNetwork{Name: "host", Driver: "host"})
		}
		if self.inspect.NetworkSettings != nil {
			port, _ := network.ParsePort(hubListenPort(cfg) + "/tcp")
			bindings := slices.Clone(self.inspect.NetworkSettings.Ports[port])
			slices.SortFunc(bindings, func(a, b network.PortBinding) int {
				if n := strings.Compare(a.HostIP.String(), b.HostIP.String()); n != 0 {
					return n
				}
				return strings.Compare(a.HostPort, b.HostPort)
			})
			for _, binding := range bindings {
				ip := net.ParseIP(binding.HostIP.String())
				if ip == nil || !validHubPort(binding.HostPort) {
					continue
				}
				number, _ := strconv.Atoi(binding.HostPort)
				published := environmenttypes.HubPublishedPort{HostIP: ip.String(), HostPort: number}
				result.Container.PublishedPorts = append(result.Container.PublishedPorts, published)
				if ip.IsUnspecified() {
					requestBindings = append(requestBindings, published)
				} else {
					addIP(ip.String(), binding.HostPort, "published")
				}
			}
		}
		slices.Sort(interfaceIPs)
		for _, ip := range interfaceIPs {
			if usableHubInterface(ip) {
				addIP(ip, hubListenPort(cfg), "interface")
			}
		}
	}
	for _, binding := range requestBindings {
		if requestIP != "" {
			addIP(requestIP, strconv.Itoa(binding.HostPort), "request")
		}
	}
	if requestHost != "" {
		add(requestScheme+requestHost, "request")
	}
	return result, nil
}
