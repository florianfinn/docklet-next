// Source: docklet-next/issues/210
// Revision: 3cba55fd10c311e4a5b3361afabb8c93fca706a0
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original shared auxiliary reservation policy regression.

package network

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCreateAuxiliaryReservationsMatchWebPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, subnet, gateway, aux string
		status                     int
	}{
		{"IPv4 network", "192.0.2.0/24", "192.0.2.1", `{"a":"192.0.2.0"}`, 400},
		{"IPv4 broadcast", "192.0.2.0/24", "192.0.2.1", `{"a":"192.0.2.255"}`, 400},
		{"IPv4 duplicate", "192.0.2.0/24", "192.0.2.1", `{"a":"192.0.2.2","b":"192.0.2.2"}`, 400},
		{"IPv6 network", "2001:db8::/64", "2001:db8::1", `{"a":"2001:db8::"}`, 400},
		{"IPv6 duplicate", "2001:db8::/64", "2001:db8::1", `{"a":"2001:db8::2","b":"2001:0DB8::2"}`, 400},
		{"IPv4 usable", "192.0.2.0/24", "192.0.2.1", `{"a":"192.0.2.2","b":"192.0.2.3"}`, 200},
		{"IPv6 last address usable", "2001:db8::/126", "2001:db8::1", `{"a":"2001:db8::3"}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &networkDockerFixture{}
			body := `{"name":"example","options":{"enableIPv6":true,"ipam":{"config":[{"subnet":"` + tc.subnet + `","gateway":"` + tc.gateway + `","auxAddress":` + tc.aux + `}]}}}`
			rec := networkRequest(networkRouter(t, f.service(t), true), "POST", "/api/environments/0/networks", body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			if tc.status == 400 {
				var problem struct {
					Code string `json:"code"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
				require.Equal(t, "network.invalid-network", problem.Code)
				require.Empty(t, f.writes)
			} else {
				require.Equal(t, []string{"create"}, f.writes)
			}
		})
	}
}
