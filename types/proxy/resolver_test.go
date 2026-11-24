// Source: docklet-next/issues/492
// Revision: 52bae4d0610f04e66451664e2c0741699c901873
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original pure M28 contract, ingress and redaction proofs.

package proxy

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func resolverRequest() ResolverRequest {
	value := "example-sentinel-$# value"
	return ResolverRequest{ResolverConfig: ResolverConfig{Name: "resolver-one", Email: "admin@example.test", CA: ResolverCA{Kind: "custom", URL: "https://ca.example.test/dir"}, Storage: "/certs/acme.json", Challenge: ResolverChallenge{Kind: "dns", Provider: HTTPRequest}}, Variables: []ResolverVariable{{Name: "HTTPREQ_ENDPOINT", Value: &value}}}
}
func TestM28ResolverRedactionAndDigest(t *testing.T) {
	q := resolverRequest()
	require.NoError(t, q.Validate())
	i := q.Redact()
	b, err := json.Marshal(i)
	require.NoError(t, err)
	require.NotContains(t, string(b), *q.Variables[0].Value)
	require.NotContains(t, string(b), `"value"`)
	require.Equal(t, "new", i.Variables[0].Mode)
	first, err := i.Digest()
	require.NoError(t, err)
	next := "different-example-value"
	q.Variables[0].Value = &next
	second, err := q.Redact().Digest()
	require.NoError(t, err)
	require.Equal(t, first, second)
	q.Variables[0].Value = nil
	require.Equal(t, "reuse", q.Redact().Variables[0].Mode)
	third, err := q.Redact().Digest()
	require.NoError(t, err)
	require.NotEqual(t, first, third)
}
func TestM28ResolverIngress(t *testing.T) {
	tests := map[string]func(*ResolverRequest){
		"name":                    func(q *ResolverRequest) { q.Name = "bad.name" },
		"name-limit":              func(q *ResolverRequest) { q.Name = strings.Repeat("a", 64) },
		"email":                   func(q *ResolverRequest) { q.Email = "Display <admin@example.test>" },
		"ca-scheme":               func(q *ResolverRequest) { q.CA.URL = "http://ca.example.test/dir" },
		"ca-userinfo":             func(q *ResolverRequest) { q.CA.URL = "https://user@ca.example.test/dir" },
		"ca-kind":                 func(q *ResolverRequest) { q.CA.Kind = "unknown" },
		"standard-ca-url":         func(q *ResolverRequest) { q.CA.Kind = "letsencrypt" },
		"storage":                 func(q *ResolverRequest) { q.Storage = "certs/acme.json" },
		"storage-clean":           func(q *ResolverRequest) { q.Storage = "/certs/../acme.json" },
		"challenge":               func(q *ResolverRequest) { q.Challenge.Kind = "tls" },
		"provider":                func(q *ResolverRequest) { q.Challenge.Provider = "other" },
		"delay-min":               func(q *ResolverRequest) { q.Challenge.DelayBeforeCheck = -1 },
		"delay-max":               func(q *ResolverRequest) { q.Challenge.DelayBeforeCheck = 601 },
		"resolvers-limit":         func(q *ResolverRequest) { q.Challenge.Resolvers = make([]string, 5) },
		"resolver-port":           func(q *ResolverRequest) { q.Challenge.Resolvers = []string{"192.0.2.53:0"} },
		"resolver-address":        func(q *ResolverRequest) { q.Challenge.Resolvers = []string{"192.0.2.53"} },
		"dns-entrypoint":          func(q *ResolverRequest) { q.Challenge.EntryPoint = "web" },
		"unknown-variable":        func(q *ResolverRequest) { q.Variables = append(q.Variables, ResolverVariable{Name: "PATH"}) },
		"other-provider-variable": func(q *ResolverRequest) { q.Variables = append(q.Variables, ResolverVariable{Name: "IONOS_API_KEY"}) },
		"missing-required":        func(q *ResolverRequest) { q.Variables = nil },
		"duplicate":               func(q *ResolverRequest) { q.Variables = append(q.Variables, q.Variables[0]) },
		"variables-limit":         func(q *ResolverRequest) { q.Variables = make([]ResolverVariable, 17) },
		"email-dollar":            func(q *ResolverRequest) { q.Email = "a$b@example.test" },
		"ca-dollar":               func(q *ResolverRequest) { q.CA.URL = "https://ca.example.test/${X}" },
		"ca-quote":                func(q *ResolverRequest) { q.CA.URL = "https://ca.example.test/a\"b" },
		"storage-newline":         func(q *ResolverRequest) { q.Storage = "/certs/acme\n.json" },
		"storage-dollar":          func(q *ResolverRequest) { q.Storage = "/certs/$X.json" },
		"resolver-host-comma":     func(q *ResolverRequest) { q.Challenge.Resolvers = []string{"192.0.2.53,192.0.2.54:53"} },
		"resolver-host-quote":     func(q *ResolverRequest) { q.Challenge.Resolvers = []string{"\"ns\".example.test:53"} },
		"alias-duplicate": func(q *ResolverRequest) {
			q.Challenge.Provider = Cloudflare
			q.Variables = []ResolverVariable{{Name: "CF_DNS_API_TOKEN"}, {Name: "CLOUDFLARE_DNS_API_TOKEN"}}
		},
	}
	for _, value := range []string{"", strings.Repeat("v", 4097), "line\nnext", "line\rnext", "a\x00b", "a'b"} {
		tests["value-"+strings.ReplaceAll(value[:min(8, len(value))], "\x00", "NUL")] = func(q *ResolverRequest) { q.Variables[0].Value = &value }
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			q := resolverRequest()
			mutate(&q)
			require.ErrorContains(t, q.Validate(), "invalid-input")
		})
	}
	q := resolverRequest()
	q.Challenge.Resolvers = []string{"192.0.2.53:53", "[2001:db8::53]:53"}
	q.Challenge.DelayBeforeCheck = 600
	require.NoError(t, q.Validate())
	q.Challenge = ResolverChallenge{Kind: "http", EntryPoint: "web"}
	q.Variables = nil
	q.CA = ResolverCA{Kind: "letsencrypt-staging"}
	require.NoError(t, q.Validate())
	q.Challenge.EntryPoint = ""
	require.Error(t, q.Validate())
	for _, entryPoint := range []string{"web secure", "web,x", "-web", "web$"} {
		q.Challenge.EntryPoint = entryPoint
		require.Error(t, q.Validate(), entryPoint)
	}
}
func TestM28ProviderRequirements(t *testing.T) {
	expected := map[DNSProvider][]string{Hetzner: {"HETZNER_API_TOKEN"}, IONOS: {"IONOS_API_KEY"}, INWX: {"INWX_USERNAME", "INWX_PASSWORD"}, Netcup: {"NETCUP_CUSTOMER_NUMBER", "NETCUP_API_KEY", "NETCUP_API_PASSWORD"}, DeSEC: {"DESEC_TOKEN"}, DuckDNS: {"DUCKDNS_TOKEN"}, HTTPRequest: {"HTTPREQ_ENDPOINT"}, Cloudflare: {"CF_DNS_API_TOKEN"}}
	require.Len(t, DNSProviders(), 8)
	for provider, names := range expected {
		t.Run(string(provider), func(t *testing.T) {
			q := resolverRequest()
			q.Challenge.Provider = provider
			q.Variables = nil
			for _, name := range names {
				q.Variables = append(q.Variables, ResolverVariable{Name: name})
			}
			require.NoError(t, q.Validate())
			q.Variables = q.Variables[1:]
			require.Error(t, q.Validate())
		})
	}
	for _, names := range [][]string{{"CF_API_EMAIL", "CF_API_KEY"}, {"CLOUDFLARE_EMAIL", "CLOUDFLARE_API_KEY"}, {"CLOUDFLARE_DNS_API_TOKEN", "CF_ZONE_API_TOKEN"}} {
		q := resolverRequest()
		q.Challenge.Provider = Cloudflare
		q.Variables = nil
		for _, name := range names {
			q.Variables = append(q.Variables, ResolverVariable{Name: name})
		}
		require.NoError(t, q.Validate())
	}
	first := DNSProviders()
	first[0].Variables[0].Name = "BROKEN"
	require.NotEqual(t, "BROKEN", DNSProviders()[0].Variables[0].Name)
}
func TestM28ResolverPhases(t *testing.T) {
	for _, phase := range []ResolverPhase{ResolverAdmitted, ResolverBackup, ResolverWriting, ResolverRecreating, ResolverObserving, ResolverApplied, ResolverRolledBack, ResolverRefused, ResolverRestoring, ResolverRecoveryRequired} {
		require.True(t, phase.Valid())
	}
	require.False(t, ResolverRecoveryRequired.Final())
	require.False(t, RecoveryRequired.Final())
	require.False(t, ResolverPhase("future").Valid())
	require.Equal(t, RouteOperation, OperationKind("").Effective())
}

func TestM28StateKindsStayDisjoint(t *testing.T) {
	for _, state := range []State{Applied, RolledBack, Refused} {
		require.True(t, state.Final())
		require.True(t, state.ValidFor(ResolverOperation))
		require.False(t, state.ValidFor(RouteOperation))
		require.False(t, state.Valid())
	}
	for _, state := range []State{Verified, AppliedUnverified, Rejected, Restored} {
		require.True(t, state.ValidFor(RouteOperation))
		require.False(t, state.ValidFor(ResolverOperation))
	}
	require.False(t, Pending.ValidFor("future"))
	for _, state := range []State{Pending, Verified, AppliedUnverified, Rejected, Restored, RolledBack, Refused, RecoveryRequired, Released, NotApplied} {
		require.False(t, state.ValidFor(AdoptionOperation), state)
	}
	require.True(t, Applied.ValidFor(AdoptionOperation))
}

func TestM28ResolverDigestBindsOperation(t *testing.T) {
	base := ResolverEnvelope{Kind: ResolverOperation, ID: "op-1", Instance: Instance{ID: "instance-1"}, Generation: "g1", Registration: "r1", Intent: resolverRequest().Redact()}
	want, err := base.Digest()
	require.NoError(t, err)
	for name, mutate := range map[string]func(*ResolverEnvelope){
		"id":           func(e *ResolverEnvelope) { e.ID = "op-2" },
		"instance":     func(e *ResolverEnvelope) { e.Instance.ID = "instance-2" },
		"generation":   func(e *ResolverEnvelope) { e.Generation = "g2" },
		"registration": func(e *ResolverEnvelope) { e.Registration = "r2" },
		"intent":       func(e *ResolverEnvelope) { e.Intent.Name = "other" },
	} {
		e := base
		mutate(&e)
		got, err := e.Digest()
		require.NoError(t, err)
		require.NotEqual(t, want, got, name)
	}
}
