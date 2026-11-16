// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original parity cases ported from the design-lab Caddyfile tests.

package caddy

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestSiteGrammarParity(t *testing.T) {
	for _, tls := range []string{"auto", "off", "custom"} {
		t.Run(tls, func(t *testing.T) {
			s := baseSite()
			s.TLS = tls
			s.Path = "/api"
			s.Upstreams = append(s.Upstreams, "http://192.0.2.81:8080")
			s.Middlewares = []string{"strip-api", "compress", "custom-headers"}
			if tls == "custom" {
				s.CertFile = "/certs/archive.crt"
				s.KeyFile = "/certs/archive.key"
			}
			text, err := renderChecked(t, s)
			require.NoError(t, err)
			require.Contains(t, text, "path /api*")
			require.NotContains(t, text, "handle_path")
			require.Contains(t, text, "lb_policy round_robin")
			got, err := parseChecked(t, text)
			require.NoError(t, err)
			require.Equal(t, s, got)
			again, err := renderChecked(t, got)
			require.NoError(t, err)
			require.Equal(t, text, again)
		})
	}
	for _, domain := range []string{"*.example.com", "media.example.com"} {
		t.Run(domain, func(t *testing.T) {
			s := baseSite()
			s.TLS = "auto"
			s.Domain = domain
			s.Resolver = "dns-example"
			s.Provider = "cloudflare"
			s.Credential = "DNS_API_TOKEN"
			s.CertificateDomain = "example.com"
			text, err := renderChecked(t, s)
			require.NoError(t, err)
			require.Contains(t, text, "dns cloudflare {env.DNS_API_TOKEN}")
			require.Contains(t, text, "example.com")
			require.Contains(t, text, "*.example.com")
			got, err := parseChecked(t, text)
			require.NoError(t, err)
			require.Equal(t, s, got)
		})
	}
	s := baseSite()
	s.Middlewares = []string{"compress"}
	text, err := renderChecked(t, s)
	require.NoError(t, err)
	changes := []func(string) string{
		func(v string) string { return v + "extra.example.net { respond ok }\n" },
		func(v string) string { return strings.ReplaceAll(v, "encode gzip", "respond ok") },
		func(v string) string { return strings.ReplaceAll(v, "encode gzip", "import extra") },
		func(v string) string { return strings.ReplaceAll(v, "encode gzip", "encode zstd") },
		func(v string) string {
			return strings.ReplaceAll(v, "docklet-route: example-app", "docklet-route: bad name")
		},
		func(v string) string { return strings.ReplaceAll(v, "# docklet-route: example-app\n", "") },
		func(v string) string {
			return strings.ReplaceAll(v, "app.example.com {", "app.example.com, other.example.com {")
		},
		func(v string) string { return strings.ReplaceAll(v, "reverse_proxy", "reverse_proxy @unknown") },
		func(v string) string { return strings.ReplaceAll(v, "encode gzip", "encode gzip\n encode gzip") },
		func(v string) string { return strings.ReplaceAll(v, s.Upstreams[0], s.Upstreams[0]+"/base") },
		func(v string) string { return v + "# unexpected metadata\n" },
		func(v string) string {
			return strings.ReplaceAll(v, "reverse_proxy "+s.Upstreams[0], "reverse_proxy "+s.Upstreams[0]+" http://192.0.2.81:8080")
		},
	}
	for n, change := range changes {
		t.Run(string(rune('a'+n)), func(t *testing.T) { _, err := parseChecked(t, change(text)); require.Error(t, err) })
	}
}
func TestSiteInvalidFields(t *testing.T) {
	changes := []func(*Site){
		func(s *Site) { s.Name = "" }, func(s *Site) { s.Domain = "example" }, func(s *Site) { s.Domain = "bad-.example.com" }, func(s *Site) { s.Domain = strings.Repeat("a", 254) + ".com" },
		func(s *Site) { s.Path = "/api { respond ok }" }, func(s *Site) { s.Upstreams = nil }, func(s *Site) { s.Upstreams = []string{"https://user:pass@example.com"} }, func(s *Site) { s.Upstreams = []string{"http://["} },
		func(s *Site) { s.Upstreams = []string{"ftp://example.com"} }, func(s *Site) { s.Upstreams = []string{"http:///"} }, func(s *Site) { s.Upstreams = []string{"http://example.com/?x"} }, func(s *Site) { s.Upstreams = []string{"http://example.com/#x"} },
		func(s *Site) { s.Upstreams = []string{"http://example.com/?"} }, func(s *Site) { s.Middlewares = []string{"unknown"} }, func(s *Site) { s.Middlewares = []string{"compress", "compress"} }, func(s *Site) { s.Resolver = "bad name" },
		func(s *Site) { s.CertificateDomain = "*.example.com" }, func(s *Site) { s.CertificateDomain = "example" }, func(s *Site) { s.Provider = "cloudflare" }, func(s *Site) { s.CertFile = "/certs/a" }, func(s *Site) { s.KeyFile = "/certs/a" },
		func(s *Site) { s.TLS = "invalid" }, func(s *Site) { s.TLS = "custom"; s.CertFile = "bad token"; s.KeyFile = "/certs/a" }, func(s *Site) { s.TLS = "custom"; s.CertFile = "/certs/a"; s.KeyFile = "" },
		func(s *Site) { s.TLS = "auto"; s.Provider = "bad-provider"; s.Credential = "TOKEN" }, func(s *Site) { s.TLS = "auto"; s.Provider = "cloudflare"; s.Credential = "bad-token" }, func(s *Site) {
			s.TLS = "auto"
			s.CertificateDomain = "example.net"
			s.Provider = "cloudflare"
			s.Credential = "TOKEN"
		},
		func(s *Site) { s.TLS = "auto"; s.Domain = "*.example.com" },
	}
	for n, change := range changes {
		t.Run(string(rune('a'+n)), func(t *testing.T) { s := baseSite(); change(&s); _, err := renderChecked(t, s); require.Error(t, err) })
	}
}

func TestSiteBoundaryAndTLSVariants(t *testing.T) {
	s := baseSite()
	s.TLS = "auto"
	text, err := renderChecked(t, s)
	require.NoError(t, err)
	require.Contains(t, text, "\napp.example.com {\n")
	require.NotContains(t, text, "*.app.example.com")
	s.Domain = "*.example.com"
	s.Provider = "cloudflare"
	s.Credential = "TOKEN"
	text, err = renderChecked(t, s)
	require.NoError(t, err)
	require.Contains(t, text, "*.example.com, example.com {")
	s.CertificateDomain = "example.net"
	_, err = renderChecked(t, s)
	require.Error(t, err)
	s = baseSite()
	s.Domain = strings.TrimSuffix(strings.Repeat(strings.Repeat("a", 63)+".", 5), ".")
	_, err = renderChecked(t, s)
	require.Error(t, err)
	s = baseSite()
	s.Upstreams = []string{"https://example.com/"}
	text, err = renderChecked(t, s)
	require.NoError(t, err)
	_, err = parseChecked(t, text)
	require.NoError(t, err)
	s.Upstreams = []string{"http://example.com/#"}
	_, err = renderChecked(t, s)
	require.Error(t, err)
	s = baseSite()
	s.Resolver = "dns-example"
	text, err = renderChecked(t, s)
	require.NoError(t, err)
	require.NotContains(t, text, "docklet-issuer")
	s.TLS = "custom"
	s.CertFile = "/certs/example.crt"
	s.KeyFile = "/certs/example.key"
	s.Provider = "cloudflare"
	s.Credential = "TOKEN"
	text, err = renderChecked(t, s)
	require.NoError(t, err)
	require.NotContains(t, text, "tls {")
	s = baseSite()
	text, err = renderChecked(t, s)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for n := 0; n < len(lines); n++ {
		_, err = parseChecked(t, strings.Join(lines[:n], "\n"))
		require.Error(t, err, "truncation %d", n)
	}
}
