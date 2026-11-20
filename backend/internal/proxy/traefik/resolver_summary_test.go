// Source: docklet-next/issues/492
// Revision: 551c24a4e4173b0e6eaa00e3ca125cca0a619124
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original guard for resolver summaries in the Traefik inventory.

package traefik

import (
	"encoding/json/v2"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

// A-R3: the inventory carries resolver summaries for the Settings view; only
// variable names and whether they are set leave the agent, never values or email.
func TestInventoryResolverSummaries(t *testing.T) {
	x := fixture(t)
	x.inspect.Config.Cmd = append(x.inspect.Config.Cmd,
		"--certificatesresolvers.web-one.acme.email=operator@example.com",
		"--certificatesresolvers.web-one.acme.storage=/certs/web.json",
		"--certificatesresolvers.web-one.acme.httpchallenge.entrypoint=web",
		"--certificatesresolvers.dns-one.acme.storage=/certs/dns.json",
		"--certificatesresolvers.dns-one.acme.caserver=https://ca.example.test/dir",
		"--certificatesresolvers.dns-one.acme.dnschallenge.provider=httpreq",
		"--certificatesresolvers.tls-one.acme.storage=/certs/tls.json",
		"--certificatesresolvers.tls-one.acme.tlschallenge=true")
	x.inspect.Config.Env = append(x.inspect.Config.Env, "HTTPREQ_ENDPOINT=http://sink.example.test", "HTTPREQ_USERNAME=")
	inv, err := x.inventory()
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"web-one", "dns-one", "tls-one"}, inv.Resolvers)
	require.Len(t, inv.ResolverSummaries, 2, "tls-one does not fit the summary contract")
	dns, web := inv.ResolverSummaries[0], inv.ResolverSummaries[1]
	require.Equal(t, "web-one", web.Name)
	require.Equal(t, dto.ResolverChallenge{Kind: "http", EntryPoint: "web"}, web.Challenge)
	require.Equal(t, dto.ResolverCA{Kind: "letsencrypt"}, web.CA)
	require.Equal(t, "/certs/web.json", web.Storage)
	require.Empty(t, web.Email)
	require.Equal(t, "dns-one", dns.Name)
	require.Equal(t, dto.ResolverCA{Kind: "custom", URL: "https://ca.example.test/dir"}, dns.CA)
	require.Equal(t, dto.HTTPRequest, dns.Challenge.Provider)
	configured := map[string]bool{}
	for _, v := range dns.Variables {
		configured[v.Name] = v.Configured
	}
	require.True(t, configured["HTTPREQ_ENDPOINT"])
	require.False(t, configured["HTTPREQ_PASSWORD"], "absent")
	require.False(t, configured["HTTPREQ_USERNAME"], "empty")
	b, err := json.Marshal(inv)
	require.NoError(t, err)
	require.NotContains(t, string(b), "operator@example.com")
	require.NotContains(t, string(b), "sink.example.test")
}
