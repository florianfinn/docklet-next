// Source: docklet-next/docs/design/network-management.md
// Revision: 81b69b33c80a454465f54a1f7289c1bc5b5a3381
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original host-side network contract implementation for issue #203.

package network

import (
	"errors"
	"net/netip"
	"strconv"
	"testing"

	"github.com/containerd/errdefs"
	networktypes "github.com/florianfinn/docklet-next/types/network"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
)

func TestDisappearingContainerDoesNotBlockWrites(t *testing.T) {
	for _, operation := range []string{"delete", "connect"} {
		t.Run(operation, func(t *testing.T) {
			f := &networkDockerFixture{networks: []network.Inspect{inspectNetwork()}, containers: []container.InspectResponse{disconnectedContainer()}, vanishing: true}
			svc := f.service(t)
			var err error
			if operation == "delete" {
				err = svc.RemoveNetwork(t.Context(), "net1", actor)
			} else {
				err = svc.ConnectContainer(t.Context(), "net1", networktypes.ConnectContainerRequest{ContainerID: "container1", IPv4Address: "192.0.2.8"}, actor)
			}
			require.NoError(t, err)
			require.Len(t, f.writes, 1)
		})
	}
}
func TestDynamicConnectDoesNotScanUnrelatedContainers(t *testing.T) {
	f := &networkDockerFixture{networks: []network.Inspect{inspectNetwork()}, containers: []container.InspectResponse{disconnectedContainer()}, failList: true}
	require.NoError(t, f.service(t).ConnectContainer(t.Context(), "net1", networktypes.ConnectContainerRequest{ContainerID: "container1", Aliases: []string{"example"}}, actor))
	require.Equal(t, []string{"connect"}, f.writes)
}

func TestDaemonWriteErrorClassification(t *testing.T) {
	raw := errors.New("synthetic daemon error")
	for _, tc := range []struct {
		name                    string
		err                     error
		conflict, invalid, code string
		status                  int
	}{
		{"remove forbidden", errdefs.ErrPermissionDenied, "in-use", "invalid-network", "in-use", 409},
		{"remove conflict", errdefs.ErrConflict, "in-use", "invalid-network", "in-use", 409},
		{"create conflict", errdefs.ErrConflict, "exists", "invalid-network", "exists", 409},
		{"disconnect conflict", errdefs.ErrConflict, "not-connected", "invalid-network", "not-connected", 409},
		{"invalid network", errdefs.ErrInvalidArgument, "in-use", "invalid-network", "invalid-network", 400},
		{"invalid address", errdefs.ErrInvalidArgument, "invalid-ip", "invalid-ip", "invalid-ip", 400},
		{"not found", errdefs.ErrNotFound, "in-use", "invalid-network", "not-found", 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := daemonWriteError(tc.err, tc.conflict, tc.invalid)
			requireCode(t, err, tc.code)
			require.Equal(t, tc.status, err.(*networktypes.Error).Status)
		})
	}
	require.Same(t, raw, daemonWriteError(raw, "in-use", "invalid-network"))
	require.ErrorIs(t, daemonWriteError(errdefs.ErrPermissionDenied, "exists", "invalid-network"), errdefs.ErrPermissionDenied)
}
func TestDaemonConflictsThroughNetworkHandlers(t *testing.T) {
	for _, status := range []int{403, 409} {
		for _, operation := range []string{"remove", "connect"} {
			t.Run(operation+"/"+strconv.Itoa(status), func(t *testing.T) {
				f := &networkDockerFixture{networks: []network.Inspect{inspectNetwork()}, containers: []container.InspectResponse{disconnectedContainer()}, writeStatus: status, writeMessage: "Address already in use"}
				method, path, body, code, want := "DELETE", "/api/environments/0/networks/net1", "", "network.in-use", 409
				if operation == "connect" {
					method, path, body, code, want = "POST", path+"/connect", `{"containerId":"container1","ipv4Address":"192.0.2.8"}`, "network.invalid-ip", 400
				}
				rec := networkRequest(networkRouter(t, f.service(t), true), method, path, body)
				require.Equal(t, want, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), `"code":"`+code+`"`)
			})
		}
	}
}

func TestFixedAddressChecksEveryPoolOfItsFamily(t *testing.T) {
	n := inspectNetwork()
	first := network.IPAMConfig{Subnet: netip.MustParsePrefix("192.0.2.0/25"), Gateway: netip.MustParseAddr("192.0.2.1")}
	second := network.IPAMConfig{Subnet: netip.MustParsePrefix("192.0.2.128/25"), Gateway: netip.MustParseAddr("192.0.2.129"), AuxAddress: map[string]netip.Addr{"reserved": netip.MustParseAddr("192.0.2.131")}}
	n.IPAM.Config = []network.IPAMConfig{first, pool6(), second}
	for _, tc := range []struct{ ip, code string }{{"192.0.2.130", ""}, {"192.0.2.2", ""}, {"192.0.2.129", "invalid-ip"}, {"192.0.2.131", "invalid-ip"}, {"192.0.2.128", "invalid-ip"}, {"192.0.2.255", "invalid-ip"}, {"198.51.100.2", "invalid-ip"}} {
		t.Run(tc.ip, func(t *testing.T) { requireCode(t, validateFixedAddress(n, nil, netip.MustParseAddr(tc.ip)), tc.code) })
	}
}
func TestCreateOverlapIgnoresDisjointPoolsAndOtherFamilies(t *testing.T) {
	for _, tc := range []struct{ name, prefix, code string }{
		{"IPv6 does not block IPv4", "2001:db8::/64", ""},
		{"disjoint IPv4", "198.51.100.0/24", ""},
		{"enclosing IPv4", "192.0.2.0/23", "overlap"},
		{"contained IPv4", "192.0.2.128/25", "overlap"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := network.Summary{IPAM: network.IPAM{Config: []network.IPAMConfig{{Subnet: netip.MustParsePrefix(tc.prefix)}}}}
			requireCode(t, validateCreate("example", options4(), []network.Summary{other}), tc.code)
		})
	}
	existing := []network.Summary{{IPAM: network.IPAM{Config: []network.IPAMConfig{pool6(), {Subnet: netip.MustParsePrefix("198.51.100.0/24")}}}}}
	require.NoError(t, validateCreate("example", options4(), existing))
	o := options4()
	o.EnableIPv4 = new(false)
	o.EnableIPv6 = new(true)
	o.IPAM.Config = []network.IPAMConfig{pool6()}
	require.NoError(t, validateCreate("example", o, []network.Summary{{IPAM: network.IPAM{Config: []network.IPAMConfig{pool4()}}}}))
}
func TestBlankWriteResourceIDsAreValidationErrors(t *testing.T) {
	for _, id := range []string{"", "  "} {
		for _, operation := range []string{"connect", "disconnect"} {
			t.Run(operation+id, func(t *testing.T) {
				f := &networkDockerFixture{networks: []network.Inspect{inspectNetwork()}}
				body := `{"containerId":"` + id + `"}`
				rec := networkRequest(networkRouter(t, f.service(t), true), "POST", "/api/environments/0/networks/net1/"+operation, body)
				require.Equal(t, 400, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), `"code":"network.invalid-network"`)
				require.Empty(t, f.writes)
			})
		}
	}
	_, err := inspectWriteNetwork(t.Context(), nil, "  ")
	requireCode(t, err, "invalid-network")
}
func TestSmallSubnetUsability(t *testing.T) {
	for _, prefix := range []string{"192.0.2.0/31", "192.0.2.1/32", "2001:db8::/128"} {
		t.Run(prefix, func(t *testing.T) {
			o := options4()
			o.EnableIPv6 = new(true)
			o.IPAM.Config = []network.IPAMConfig{{Subnet: netip.MustParsePrefix(prefix)}}
			err := validateCreate("example", o, nil)
			requireCode(t, err, "invalid-network")
			require.ErrorContains(t, err, "no usable")
			n := inspectNetwork()
			n.IPAM.Config = o.IPAM.Config
			requireCode(t, validateFixedAddress(n, nil, o.IPAM.Config[0].Subnet.Addr()), "invalid-ip")
		})
	}
	n := inspectNetwork()
	n.IPAM.Config = []network.IPAMConfig{{Subnet: netip.MustParsePrefix("2001:db8::/127")}}
	require.NoError(t, validateFixedAddress(n, nil, netip.MustParseAddr("2001:db8::1")))
	requireCode(t, validateFixedAddress(n, nil, netip.MustParseAddr("2001:db8::")), "invalid-ip")
	n.IPAM.Config[0].Gateway = netip.MustParseAddr("2001:db8::1")
	requireCode(t, validateFixedAddress(n, nil, netip.MustParseAddr("2001:db8::1")), "invalid-ip")
	o := options4()
	o.EnableIPv6 = new(true)
	o.IPAM.Config = n.IPAM.Config
	require.NoError(t, validateCreate("example", o, nil))
}

func TestPruneRemovalPreservesNotFoundClassification(t *testing.T) {
	f := &networkDockerFixture{writeStatus: 404}
	err := f.service(t).RemoveNetworkForPrune(t.Context(), "gone", actor)
	require.True(t, errdefs.IsNotFound(err), "scheduled prune must recognize a concurrently removed target: %v", err)
}

func TestReferenceInspectFailureBlocksWrites(t *testing.T) {
	for _, operation := range []string{"delete", "connect"} {
		t.Run(operation, func(t *testing.T) {
			f := &networkDockerFixture{networks: []network.Inspect{inspectNetwork()}, containers: []container.InspectResponse{disconnectedContainer()}, inspectStatus: 500}
			method, path, body := "DELETE", "/api/environments/0/networks/net1", ""
			if operation == "connect" {
				method, path, body = "POST", path+"/connect", `{"containerId":"container1","ipv4Address":"192.0.2.8"}`
			}
			rec := networkRequest(networkRouter(t, f.service(t), true), method, path, body)
			require.Equal(t, 500, rec.Code, rec.Body.String())
			require.Empty(t, f.writes)
			require.NotContains(t, rec.Body.String(), "inspect failed")
		})
	}
}

func TestConcurrentConnectConflictsThroughHandler(t *testing.T) {
	for _, status := range []int{403, 409} {
		for _, tc := range []struct {
			name, message, code string
			want                int
		}{
			{"duplicate", "endpoint with name example already exists in network example", "network.already-connected", 409},
			{"attached", "example is already attached to network example", "network.already-connected", 409},
			{"address", "Address already in use", "network.invalid-ip", 400},
			{"unknown", "synthetic unknown conflict", "", 500},
		} {
			t.Run(tc.name+"/"+strconv.Itoa(status), func(t *testing.T) {
				f := &networkDockerFixture{networks: []network.Inspect{inspectNetwork()}, containers: []container.InspectResponse{disconnectedContainer()}, writeStatus: status, writeMessage: tc.message}
				rec := networkRequest(networkRouter(t, f.service(t), true), "POST", "/api/environments/0/networks/net1/connect", `{"containerId":"container1"}`)
				require.Equal(t, tc.want, rec.Code, rec.Body.String())
				if tc.code != "" {
					require.Contains(t, rec.Body.String(), `"code":"`+tc.code+`"`)
				}
				require.NotContains(t, rec.Body.String(), tc.message)
			})
		}
	}
}

func TestProtectedDeleteThroughHandler(t *testing.T) {
	for _, tc := range []struct{ name, label, code string }{
		{"bridge", "", "builtin-network"}, {"host", "", "builtin-network"}, {"none", "", "builtin-network"}, {"ingress", "", "builtin-network"},
		{"example", "com.docker.compose.project", "compose-managed"}, {"example", "com.docker.compose.network", "compose-managed"},
	} {
		t.Run(tc.name+tc.label, func(t *testing.T) {
			n := inspectNetwork()
			n.Name = tc.name
			if tc.label != "" {
				n.Labels = map[string]string{tc.label: "example"}
			}
			f := &networkDockerFixture{networks: []network.Inspect{n}}
			rec := networkRequest(networkRouter(t, f.service(t), true), "DELETE", "/api/environments/0/networks/net1", "")
			require.Equal(t, 409, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), `"code":"network.`+tc.code+`"`)
			require.Empty(t, f.writes)
		})
	}
}
