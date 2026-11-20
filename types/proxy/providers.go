// Source: docklet-next/issues/492
// Revision: 52bae4d0610f04e66451664e2c0741699c901873
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 contract foundation; no proxy effects.

package proxy

// Variable names match lego v5.5.2, used by Traefik v3.7.16. Sources:
// https://github.com/traefik/traefik/blob/v3.7.16/go.mod
// https://github.com/go-acme/lego/tree/v5.5.2/providers/dns
// Only direct-value names are curated; file indirection is not a resolver input.
type DNSProvider string

const (
	Cloudflare  DNSProvider = "cloudflare"
	Hetzner     DNSProvider = "hetzner"
	IONOS       DNSProvider = "ionos"
	INWX        DNSProvider = "inwx"
	Netcup      DNSProvider = "netcup"
	DeSEC       DNSProvider = "desec"
	DuckDNS     DNSProvider = "duckdns"
	HTTPRequest DNSProvider = "httpreq"
)

type DNSProviderVariable struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	AliasOf  string `json:"aliasOf,omitempty"`
}
type DNSProviderDefinition struct {
	Provider  DNSProvider           `json:"provider" enum:"cloudflare,hetzner,ionos,inwx,netcup,desec,duckdns,httpreq"`
	Variables []DNSProviderVariable `json:"variables"`
	// CredentialSets are alternatives, after resolving AliasOf names.
	CredentialSets [][]string `json:"credentialSets"`
}

// DNSProviders returns fresh tables so callers cannot mutate validation rules.
func DNSProviders() []DNSProviderDefinition {
	return []DNSProviderDefinition{
		{Provider: Cloudflare, Variables: []DNSProviderVariable{
			{Name: "CF_API_EMAIL", Required: false},
			{Name: "CF_API_KEY", Required: false},
			{Name: "CF_DNS_API_TOKEN", Required: false},
			{Name: "CF_ZONE_API_TOKEN", Required: false},
			{Name: "CLOUDFLARE_EMAIL", Required: false, AliasOf: "CF_API_EMAIL"},
			{Name: "CLOUDFLARE_API_KEY", Required: false, AliasOf: "CF_API_KEY"},
			{Name: "CLOUDFLARE_DNS_API_TOKEN", Required: false, AliasOf: "CF_DNS_API_TOKEN"},
			{Name: "CLOUDFLARE_ZONE_API_TOKEN", Required: false, AliasOf: "CF_ZONE_API_TOKEN"},
			{Name: "CLOUDFLARE_POLLING_INTERVAL", Required: false},
			{Name: "CLOUDFLARE_PROPAGATION_TIMEOUT", Required: false},
			{Name: "CLOUDFLARE_TTL", Required: false},
			{Name: "CLOUDFLARE_HTTP_TIMEOUT", Required: false},
			{Name: "CLOUDFLARE_BASE_URL", Required: false},
		}, CredentialSets: [][]string{{"CF_DNS_API_TOKEN"}, {"CF_API_EMAIL", "CF_API_KEY"}}},
		{Provider: Hetzner, Variables: []DNSProviderVariable{
			{Name: "HETZNER_API_TOKEN", Required: true},
			{Name: "HETZNER_POLLING_INTERVAL", Required: false},
			{Name: "HETZNER_PROPAGATION_TIMEOUT", Required: false},
			{Name: "HETZNER_TTL", Required: false},
			{Name: "HETZNER_HTTP_TIMEOUT", Required: false},
		}, CredentialSets: [][]string{{"HETZNER_API_TOKEN"}}},
		{Provider: IONOS, Variables: []DNSProviderVariable{
			{Name: "IONOS_API_KEY", Required: true},
			{Name: "IONOS_POLLING_INTERVAL", Required: false},
			{Name: "IONOS_PROPAGATION_TIMEOUT", Required: false},
			{Name: "IONOS_TTL", Required: false},
			{Name: "IONOS_HTTP_TIMEOUT", Required: false},
		}, CredentialSets: [][]string{{"IONOS_API_KEY"}}},
		{Provider: INWX, Variables: []DNSProviderVariable{
			{Name: "INWX_USERNAME", Required: true},
			{Name: "INWX_PASSWORD", Required: true},
			{Name: "INWX_SHARED_SECRET", Required: false},
			{Name: "INWX_POLLING_INTERVAL", Required: false},
			{Name: "INWX_PROPAGATION_TIMEOUT", Required: false},
			{Name: "INWX_TTL", Required: false},
			{Name: "INWX_SANDBOX", Required: false},
		}, CredentialSets: [][]string{{"INWX_USERNAME", "INWX_PASSWORD"}}},
		{Provider: Netcup, Variables: []DNSProviderVariable{
			{Name: "NETCUP_CUSTOMER_NUMBER", Required: true},
			{Name: "NETCUP_API_KEY", Required: true},
			{Name: "NETCUP_API_PASSWORD", Required: true},
			{Name: "NETCUP_POLLING_INTERVAL", Required: false},
			{Name: "NETCUP_PROPAGATION_TIMEOUT", Required: false},
			{Name: "NETCUP_HTTP_TIMEOUT", Required: false},
		}, CredentialSets: [][]string{{"NETCUP_CUSTOMER_NUMBER", "NETCUP_API_KEY", "NETCUP_API_PASSWORD"}}},
		{Provider: DeSEC, Variables: []DNSProviderVariable{
			{Name: "DESEC_TOKEN", Required: true},
			{Name: "DESEC_POLLING_INTERVAL", Required: false},
			{Name: "DESEC_PROPAGATION_TIMEOUT", Required: false},
			{Name: "DESEC_TTL", Required: false},
			{Name: "DESEC_HTTP_TIMEOUT", Required: false},
		}, CredentialSets: [][]string{{"DESEC_TOKEN"}}},
		{Provider: DuckDNS, Variables: []DNSProviderVariable{
			{Name: "DUCKDNS_TOKEN", Required: true},
			{Name: "DUCKDNS_POLLING_INTERVAL", Required: false},
			{Name: "DUCKDNS_PROPAGATION_TIMEOUT", Required: false},
			{Name: "DUCKDNS_HTTP_TIMEOUT", Required: false},
			{Name: "DUCKDNS_SEQUENCE_INTERVAL", Required: false},
		}, CredentialSets: [][]string{{"DUCKDNS_TOKEN"}}},
		{Provider: HTTPRequest, Variables: []DNSProviderVariable{
			{Name: "HTTPREQ_MODE", Required: false},
			{Name: "HTTPREQ_ENDPOINT", Required: true},
			{Name: "HTTPREQ_USERNAME", Required: false},
			{Name: "HTTPREQ_PASSWORD", Required: false},
			{Name: "HTTPREQ_POLLING_INTERVAL", Required: false},
			{Name: "HTTPREQ_PROPAGATION_TIMEOUT", Required: false},
			{Name: "HTTPREQ_HTTP_TIMEOUT", Required: false},
		}, CredentialSets: [][]string{{"HTTPREQ_ENDPOINT"}}},
	}
}
