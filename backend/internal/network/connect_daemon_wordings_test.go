// Source: docklet-next/issues/207
// Revision: 1c1597f131fdbd87d6043a44fa32b7d2149388f3
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Adopt independent-review daemon wording regression cases.

package network

import (
	"strconv"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
)

// Wordings from moby b711d21: libnetwork wraps IPAM failures; other 403s are not address conflicts.
func TestConnectDaemonWordings(t *testing.T) {
	for _, status := range []int{403, 409} {
		for _, tc := range []struct {
			name, message, code string
			want                int
		}{
			{"wrapped-address", "failed to set up container networking: Address already in use", "network.invalid-ip", 400},
			{"shared-endpoint", "another container is attached to the same network endpoint", "", 500},
			{"stopped-attached", "example is already attached to network example_net", "network.already-connected", 409},
		} {
			t.Run(tc.name+"/"+strconv.Itoa(status), func(t *testing.T) {
				f := &networkDockerFixture{networks: []network.Inspect{inspectNetwork()}, containers: []container.InspectResponse{disconnectedContainer()}, writeStatus: status, writeMessage: tc.message}
				rec := networkRequest(networkRouter(t, f.service(t), true), "POST", "/api/environments/0/networks/net1/connect", `{"containerId":"container1","ipv4Address":"192.0.2.8"}`)
				require.Equal(t, tc.want, rec.Code, rec.Body.String())
				if tc.code != "" {
					require.Contains(t, rec.Body.String(), `"code":"`+tc.code+`"`)
				} else {
					require.NotContains(t, rec.Body.String(), `"code":"network.`)
				}
				require.NotContains(t, rec.Body.String(), tc.message)
			})
		}
	}
}
