// Source: docklet-next/issues/519
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original real-shaped inspect tables, request transport and serialisation tests.

package environment

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	environmenttypes "github.com/florianfinn/docklet-next/types/environment"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
)

func hostHubSelf() *hubSelf {
	return &hubSelf{inspect: container.InspectResponse{Name: "/hub", HostConfig: &container.HostConfig{NetworkMode: "host"}}}
}

func bridgeHubSelf() *hubSelf {
	return &hubSelf{inspect: container.InspectResponse{ID: "hub-id", Name: "/hub", HostConfig: &container.HostConfig{NetworkMode: "example-network"}, NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{"example-network": {NetworkID: "net-id", IPAddress: netip.MustParseAddr("192.0.2.200")}}, Ports: network.PortMap{}}}, drivers: map[string]string{"example-network": "bridge"}}
}

func candidate(url, source, kind, reach string, origins ...string) environmenttypes.HubAddressCandidate {
	if len(origins) == 0 {
		origins = []string{source}
	}
	return environmenttypes.HubAddressCandidate{ID: source + ":" + url, URL: url, Source: source, Origins: origins, Kind: kind, Reach: reach}
}

func TestHubCandidatesNetworkShapes(t *testing.T) {
	ipCandidate := func(url, source string) environmenttypes.HubAddressCandidate {
		return candidate(url, source, "ip", "unknown")
	}
	for _, tc := range []struct {
		name  string
		setup func(*hubSelf)
		host  string
		want  []environmenttypes.HubAddressCandidate
	}{
		{"bridge wildcard", func(s *hubSelf) {
			s.inspect.NetworkSettings.Ports[network.MustParsePort("3552/tcp")] = []network.PortBinding{{HostIP: netip.IPv4Unspecified(), HostPort: "4552"}}
		}, "192.0.2.10:3552", []environmenttypes.HubAddressCandidate{ipCandidate("http://192.0.2.10:4552", "request"), ipCandidate("http://192.0.2.10:3552", "request")}},
		{"bridge loopback", func(s *hubSelf) {
			s.inspect.NetworkSettings.Ports[network.MustParsePort("3552/tcp")] = []network.PortBinding{{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: "4552"}}
		}, "", []environmenttypes.HubAddressCandidate{candidate("http://127.0.0.1:4552", "published", "ip", "same-host")}},
		{"bridge specific binding", func(s *hubSelf) {
			s.inspect.NetworkSettings.Ports[network.MustParsePort("3552/tcp")] = []network.PortBinding{{HostIP: netip.MustParseAddr("192.0.2.10"), HostPort: "4552"}}
		}, "", []environmenttypes.HubAddressCandidate{ipCandidate("http://192.0.2.10:4552", "published")}},
		{"bridge no binding", func(*hubSelf) {}, "", []environmenttypes.HubAddressCandidate{}},
		{"host mode", func(s *hubSelf) {
			s.inspect.HostConfig.NetworkMode = "host"
			s.inspect.NetworkSettings = &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{"host": {}}}
			s.drivers = map[string]string{"host": "host"}
		}, "", []environmenttypes.HubAddressCandidate{ipCandidate("http://192.0.2.30:3552", "interface"), ipCandidate("http://[2001:db8::30]:3552", "interface")}},
		{"host driver", func(s *hubSelf) { s.drivers["example-network"] = "host" }, "", []environmenttypes.HubAddressCandidate{ipCandidate("http://192.0.2.30:3552", "interface"), ipCandidate("http://[2001:db8::30]:3552", "interface")}},
		{"macvlan", func(s *hubSelf) { s.drivers["example-network"] = "macvlan" }, "", []environmenttypes.HubAddressCandidate{ipCandidate("http://192.0.2.200:3552", "interface")}},
		{"ipvlan", func(s *hubSelf) { s.drivers["example-network"] = "ipvlan" }, "", []environmenttypes.HubAddressCandidate{ipCandidate("http://192.0.2.200:3552", "interface")}},
		{"mixed bridge macvlan", func(s *hubSelf) {
			s.inspect.NetworkSettings.Networks["lan"] = &network.EndpointSettings{IPAddress: netip.MustParseAddr("192.0.2.40"), GlobalIPv6Address: netip.MustParseAddr("2001:db8::40")}
			s.drivers["lan"] = "macvlan"
		}, "", []environmenttypes.HubAddressCandidate{ipCandidate("http://192.0.2.40:3552", "interface"), ipCandidate("http://[2001:db8::40]:3552", "interface")}},
		{"named user bridge", func(s *hubSelf) { s.inspect.HostConfig.NetworkMode = "example-network" }, "hub.example.com", []environmenttypes.HubAddressCandidate{candidate("http://hub.example.com", "request", "domain", "unknown")}},
		{"overlay", func(s *hubSelf) { s.drivers["example-network"] = "overlay" }, "", []environmenttypes.HubAddressCandidate{}},
		{"other driver", func(s *hubSelf) { s.drivers["example-network"] = "other" }, "", []environmenttypes.HubAddressCandidate{}},
		{"IPv6 wildcard", func(s *hubSelf) {
			s.inspect.NetworkSettings.Ports[network.MustParsePort("3552/tcp")] = []network.PortBinding{{HostIP: netip.IPv6Unspecified(), HostPort: "4552"}}
		}, "[2001:db8::10]:3552", []environmenttypes.HubAddressCandidate{ipCandidate("http://[2001:db8::10]:4552", "request"), ipCandidate("http://[2001:db8::10]:3552", "request")}},
		{"IPv6 specific", func(s *hubSelf) {
			s.inspect.NetworkSettings.Ports[network.MustParsePort("3552/tcp")] = []network.PortBinding{{HostIP: netip.MustParseAddr("2001:db8::10"), HostPort: "4552"}}
		}, "", []environmenttypes.HubAddressCandidate{ipCandidate("http://[2001:db8::10]:4552", "published")}},
		{"domain with wildcard", func(s *hubSelf) {
			s.inspect.NetworkSettings.Ports[network.MustParsePort("3552/tcp")] = []network.PortBinding{{HostIP: netip.IPv4Unspecified(), HostPort: "4552"}}
		}, "hub.example.com:443", []environmenttypes.HubAddressCandidate{candidate("http://hub.example.com:443", "request", "domain", "unknown")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			self := bridgeHubSelf()
			tc.setup(self)
			result, err := buildHubDiscovery(&config.Config{}, "", "", self, []string{"192.0.2.30", "2001:db8::30", "127.0.0.1", "::1", "0.0.0.0", "::", net.IPv6linklocalallnodes.String(), "invalid"}, tc.host)
			require.NoError(t, err)
			require.Equal(t, tc.want, result.Candidates)
			require.Equal(t, "container", result.ContainerDetection)
			require.Equal(t, "hub", result.Container.Name)
			require.Equal(t, 3552, result.Container.ListenPort)
			require.Equal(t, self.drivers[result.Container.Networks[0].Name], result.Container.Networks[0].Driver)
		})
	}
}

func TestHubCandidatesOrderAndDeduplication(t *testing.T) {
	self := bridgeHubSelf()
	self.drivers["example-network"] = "macvlan"
	self.inspect.NetworkSettings.Ports[network.MustParsePort("3552/tcp")] = []network.PortBinding{{HostIP: netip.MustParseAddr("192.0.2.10"), HostPort: "4552"}}
	result, err := buildHubDiscovery(&config.Config{AppUrl: "https://app.example.com"}, "https://public.example.com/", "192.0.2.5:3552", self, nil, "192.0.2.200:3552")
	require.NoError(t, err)
	require.Equal(t, []environmenttypes.HubAddressCandidate{
		candidate("https://public.example.com", "configured", "domain", "unknown", "settingsDomain"),
		candidate("https://app.example.com", "configured", "domain", "unknown", "appUrl"),
		candidate("http://192.0.2.5:3552", "configured", "ip", "unknown", "settingsAddress"),
		candidate("http://192.0.2.10:4552", "published", "ip", "unknown"),
		candidate("http://192.0.2.200:3552", "interface", "ip", "unknown", "interface", "request"),
	}, result.Candidates)
}

// The same URL from settings, APP_URL and the request is one candidate that names every origin.
func TestHubCandidatesMergeOrigins(t *testing.T) {
	result, err := buildHubDiscovery(&config.Config{AppUrl: "https://hub.example.com"}, "https://hub.example.com/", "hub.example.com", nil, nil, "https://hub.example.com")
	require.NoError(t, err)
	require.Equal(t, []environmenttypes.HubAddressCandidate{
		candidate("https://hub.example.com", "configured", "domain", "unknown", "settingsDomain", "appUrl", "request"),
		candidate("http://hub.example.com", "configured", "domain", "unknown", "settingsAddress"),
	}, result.Candidates)
}

func TestHubDiscoveryRequestHTTP(t *testing.T) {
	settingsService, err := settings.NewSettingsService(t.Context(), setupEnvironmentServiceTestDB(t))
	require.NoError(t, err)
	for _, tc := range []struct{ name, peer, trusted, host, forwarded, want string }{
		{"untrusted spoof", "192.0.2.20:1234", "192.0.2.10/32", "hub.example.com:3552", "spoof.example.com", "http://hub.example.com:3552"},
		{"trusted proxy", "192.0.2.10:1234", "invalid, 192.0.2.10/32", "hub.example.com:3552", "public.example.com", "http://public.example.com"},
		{"trusted TLS proxy", "192.0.2.10:1234", "192.0.2.10/32", "hub.example.com:3552", "public.example.com|https", "https://public.example.com"},
		{"untrusted proto spoof", "192.0.2.20:1234", "192.0.2.10/32", "hub.example.com:3552", "|https", "http://hub.example.com:3552"},
		{"trusted IPv6", "[2001:db8::10]:1234", "2001:db8::/32", "hub.example.com", "[2001:db8::20]:4552", "http://[2001:db8::20]:4552"},
		{"malformed host", "192.0.2.10:1234", "", "bad host", "", ""},
		{"malformed forwarded list", "192.0.2.10:1234", "192.0.2.10/32", "hub.example.com", "public.example.com, spoof.example.com", ""},
		{"malformed forwarded port", "192.0.2.10:1234", "192.0.2.10/32", "hub.example.com", "public.example.com:99999", ""},
		{"userinfo host", "192.0.2.10:1234", "", "operator@hub.example.com", "", ""},
		{"unbracketed IPv6", "192.0.2.10:1234", "", "2001:db8::20", "", ""},
		{"unparseable peer", "invalid", "192.0.2.10/32", "hub.example.com", "spoof.example.com", "http://hub.example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := echo.New()
			api := humaecho.New(router, huma.DefaultConfig("Discovery", "0.0.0"))
			h := &EnvironmentHandler{settingsService: settingsService, cfg: &config.Config{TrustedProxies: tc.trusted}}
			huma.Register(api, huma.Operation{OperationID: "discoverHub", Method: "GET", Path: "/hub/discovery"}, h.DiscoverHub)
			req := httptest.NewRequest("GET", "/hub/discovery", nil)
			req.Host, req.RemoteAddr = tc.host, tc.peer
			forwarded, proto, _ := strings.Cut(tc.forwarded, "|")
			req.Header.Set("X-Forwarded-Host", forwarded)
			req.Header.Set("X-Forwarded-Proto", proto)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			var result base.ApiResponse[environmenttypes.HubDiscovery]
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
			require.Equal(t, "unknown", result.Data.ContainerDetection)
			require.Nil(t, result.Data.Container)
			if tc.want == "" {
				require.Empty(t, result.Data.Candidates)
			} else {
				require.Len(t, result.Data.Candidates, 1)
				require.Equal(t, tc.want, result.Data.Candidates[0].URL)
			}
		})
	}
}

func TestHubDiscoveryWireFields(t *testing.T) {
	self := bridgeHubSelf()
	self.inspect.NetworkSettings.Ports[network.MustParsePort("3552/tcp")] = []network.PortBinding{{HostIP: netip.MustParseAddr("192.0.2.10"), HostPort: "4552"}}
	result, err := buildHubDiscovery(&config.Config{}, "", "", self, nil, "hub.example.com")
	require.NoError(t, err)
	raw, err := json.Marshal(handlerutil.Out[environmenttypes.HubDiscovery]{Body: base.ApiResponse[environmenttypes.HubDiscovery]{Success: true, Data: result}}.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"name":"hub","listenPort":3552,"networks":[{"name":"example-network","driver":"bridge"}],"publishedPorts":[{"hostIp":"192.0.2.10","hostPort":4552}]}`, string(mustJSONField(t, mustJSONField(t, raw, "data"), "container")))
	require.Equal(t, `"container"`, string(mustJSONField(t, mustJSONField(t, raw, "data"), "containerDetection")))
	var decoded base.ApiResponse[environmenttypes.HubDiscovery]
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, "published", decoded.Data.Candidates[0].Source)
	require.Equal(t, "ip", decoded.Data.Candidates[0].Kind)
	require.Equal(t, "request", decoded.Data.Candidates[1].Source)
	require.Equal(t, "domain", decoded.Data.Candidates[1].Kind)
}

func mustJSONField(t *testing.T, raw []byte, field string) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	require.Contains(t, fields, field)
	return fields[field]
}

// Configured addresses keep the acceptance they had before discovery became container-aware.
func TestHubDiscoveryKeepsConfiguredNames(t *testing.T) {
	result, err := buildHubDiscovery(&config.Config{}, "https://bücher.example", "docklet_hub:3552", nil, nil, "")
	require.NoError(t, err)
	urls := []string{}
	for _, candidate := range result.Candidates {
		urls = append(urls, candidate.URL)
	}
	require.Equal(t, []string{"https://b%C3%BCcher.example", "http://docklet_hub:3552"}, urls)
}

// Equivalent spellings merge: domain case and default port, IP candidates across schemes, IPv6 spelling.
func TestHubCandidatesMergeEquivalentAddresses(t *testing.T) {
	result, err := buildHubDiscovery(&config.Config{AppUrl: "https://hub.example.com:443"}, "https://HUB.example.com/", "192.0.2.10:3552", nil, nil, "https://192.0.2.10:3552")
	require.NoError(t, err)
	require.Equal(t, []environmenttypes.HubAddressCandidate{
		candidate("https://HUB.example.com", "configured", "domain", "unknown", "settingsDomain", "appUrl"),
		candidate("http://192.0.2.10:3552", "configured", "ip", "unknown", "settingsAddress", "request"),
	}, result.Candidates)
	v6, err := buildHubDiscovery(&config.Config{AppUrl: "http://[2001:db8:0:0::10]:3552"}, "", "[2001:db8::10]:3552", nil, nil, "")
	require.NoError(t, err)
	require.Len(t, v6.Candidates, 1)
	require.Equal(t, []string{"appUrl", "settingsAddress"}, v6.Candidates[0].Origins)
}

// A stored bare IPv6 hub address becomes a bracketed IP candidate, not a malformed domain.
func TestHubCandidatesBareIPv6Address(t *testing.T) {
	for address, want := range map[string]string{"2001:db8::10": "http://[2001:db8::10]", "::ffff:192.0.2.10": "http://[::ffff:192.0.2.10]", "::ffff:c000:20a": "http://[::ffff:c000:20a]"} {
		result, err := buildHubDiscovery(&config.Config{}, "", address, nil, nil, "")
		require.NoError(t, err)
		require.Equal(t, []environmenttypes.HubAddressCandidate{candidate(want, "configured", "ip", "unknown", "settingsAddress")}, result.Candidates, address)
	}
}
