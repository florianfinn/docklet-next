// Source: docklet-next/issues/85
// Revision: 37ddf907edabe127fe061523c7aba1c764d87d85
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original acceptance correction and regression coverage.

package container

import (
	containertypes "github.com/florianfinn/docklet-next/types/container"
	dockercontainer "github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"net/netip"
	"testing"
)

func TestD6MergeAddressFamilyBindings(t *testing.T) {
	input := []containertypes.Port{{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"}, {IP: "::", PrivatePort: 80, PublicPort: 8080, Type: "tcp"}, {PrivatePort: 80, PublicPort: 8080, Type: "udp"}, {PrivatePort: 80, PublicPort: 8081, Type: "tcp"}}
	merged := mergePortBindings(input)
	require.Len(t, merged, 3)
	require.Empty(t, merged[0].IP)
	require.Equal(t, "udp", merged[1].Type)
	service := &ContainerService{}
	summaries := service.BuildSummaries(t.Context(), []dockercontainer.Summary{{ID: "example", Image: "example:1.0.0", Ports: []dockercontainer.PortSummary{{IP: netip.MustParseAddr("0.0.0.0"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"}, {IP: netip.MustParseAddr("::"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"}}}}, nil, "", nil)
	require.Len(t, summaries[0].Ports, 1)
}

func TestAN3ExplicitPortAddressesRemainDistinct(t *testing.T) {
	for _, addresses := range [][]string{
		{"0.0.0.0", "::", "127.0.0.1", "192.0.2.10", "2001:db8::10", ""},
		{"::", "0.0.0.0", "2001:db8::10", "192.0.2.10", "127.0.0.1", ""},
	} {
		ports := make([]containertypes.Port, 0, len(addresses))
		for _, address := range addresses {
			ports = append(ports, containertypes.Port{IP: address, PrivatePort: 80, PublicPort: 8080, Type: "tcp"})
		}
		merged := mergePortBindings(ports)
		require.Len(t, merged, 5)
		require.Empty(t, merged[0].IP)
		require.Equal(t, ports[2:], merged[1:])
	}
	for _, address := range []string{"0.0.0.0", "::", "192.0.2.10", "2001:db8::10", ""} {
		port := containertypes.Port{IP: address, PrivatePort: 80, PublicPort: 8080, Type: "tcp"}
		require.Equal(t, []containertypes.Port{port}, mergePortBindings([]containertypes.Port{port}))
	}
}
