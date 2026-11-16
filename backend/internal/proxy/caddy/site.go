// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Go port of the bounded design-lab Caddyfile grammar.

package caddy

import (
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

// Site is the complete editable grammar. Credential stores an environment name,
// never a credential value. ParseSite refuses everything Render cannot retain.
type Site struct {
	Name              string   `json:"name"`
	Domain            string   `json:"domain"`
	Path              string   `json:"path"`
	TLS               string   `json:"tls"`
	Resolver          string   `json:"resolver,omitempty"`
	CertificateDomain string   `json:"certificateDomain,omitempty"`
	Provider          string   `json:"provider,omitempty"`
	Credential        string   `json:"credential,omitempty"`
	CertFile          string   `json:"certFile,omitempty"`
	KeyFile           string   `json:"keyFile,omitempty"`
	Upstreams         []string `json:"upstreams"`
	Middlewares       []string `json:"middlewares"`
}

// renameOnly reports sites that differ at most in their name comment.
func renameOnly(old, next Site) bool {
	old.Name, next.Name = "", ""
	return reflect.DeepEqual(old, next)
}

var routeName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,79}$`)
var domainLabel = regexp.MustCompile(`(?i)^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var pathValue = regexp.MustCompile(`(?i)^/[a-z0-9/_.~%-]*$`)
var providerName = regexp.MustCompile(`^[a-z0-9_]+$`)
var envName = regexp.MustCompile(`(?i)^[A-Z_][A-Z0-9_]*$`)
var token = regexp.MustCompile("^[^\\s{}#\"`]+$")
var middlewareLines = map[string]string{"compress": "encode gzip", "strip-api": "uri strip_prefix /api", "custom-headers": "header X-Proxy demo"}

func validDomain(v string) bool {
	v = strings.TrimPrefix(v, "*.")
	if len(v) > 253 || !strings.Contains(v, ".") {
		return false
	}
	for _, p := range strings.Split(v, ".") {
		if !domainLabel.MatchString(p) {
			return false
		}
	}
	return true
}
func upstream(v string) bool {
	u, err := url.Parse(v)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && (u.Path == "" || u.Path == "/") && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && token.MatchString(v)
}
func (s Site) addresses() []string {
	a := []string{s.Domain}
	if s.TLS == "auto" {
		wildcard := s.CertificateDomain
		if wildcard == "" && strings.HasPrefix(s.Domain, "*.") {
			wildcard = strings.TrimPrefix(s.Domain, "*.")
		}
		if wildcard != "" {
			for _, d := range []string{wildcard, "*." + wildcard} {
				if !slices.Contains(a, d) {
					a = append(a, d)
				}
			}
		}
	}
	return a
}
func invalidSite() error { return fmt.Errorf("unsupported Caddy site grammar") }
func (s Site) valid() error {
	if !routeName.MatchString(s.Name) || !validDomain(s.Domain) || s.Path != "" && !pathValue.MatchString(s.Path) || len(s.Upstreams) == 0 {
		return invalidSite()
	}
	if s.Resolver != "" && !routeName.MatchString(s.Resolver) {
		return invalidSite()
	}
	if s.CertificateDomain != "" && (!validDomain(s.CertificateDomain) || strings.HasPrefix(s.CertificateDomain, "*.")) {
		return invalidSite()
	}
	for _, u := range s.Upstreams {
		if !upstream(u) {
			return invalidSite()
		}
	}
	seen := map[string]bool{}
	for _, m := range s.Middlewares {
		if middlewareLines[m] == "" || seen[m] {
			return invalidSite()
		}
		seen[m] = true
	}
	switch s.TLS {
	case "off":
		if s.Provider != "" || s.CertFile != "" || s.KeyFile != "" {
			return invalidSite()
		}
	case "auto":
		if s.Provider != "" && (!providerName.MatchString(s.Provider) || !envName.MatchString(s.Credential)) {
			return invalidSite()
		}
		wildcard := s.CertificateDomain
		if wildcard == "" && strings.HasPrefix(s.Domain, "*.") {
			wildcard = strings.TrimPrefix(s.Domain, "*.")
		}
		if wildcard != "" {
			if s.Provider == "" || !(s.Domain == wildcard || strings.HasSuffix(s.Domain, "."+wildcard) && len(strings.Split(s.Domain, ".")) == len(strings.Split(wildcard, "."))+1) {
				return invalidSite()
			}
		}
	case "custom":
		if !token.MatchString(s.CertFile) || !token.MatchString(s.KeyFile) {
			return invalidSite()
		}
	default:
		return invalidSite()
	}
	return nil
}

// Render generates the same ordered handlers and explicit non-stripping matcher
// as web/src/features/design-lab/proxy/caddyfile.ts.
func (s Site) Render() (string, error) {
	if err := s.valid(); err != nil {
		return "", err
	}
	lines := []string{"# docklet-route: " + s.Name}
	if s.Resolver != "" && s.TLS == "auto" {
		lines = append(lines, "# docklet-issuer: "+s.Resolver)
	}
	if s.CertificateDomain != "" {
		lines = append(lines, "# docklet-certificate-domain: "+s.CertificateDomain)
	}
	a := s.addresses()
	prefix := ""
	if s.TLS == "off" {
		prefix = "http://"
	}
	lines = append(lines, prefix+strings.Join(a, ", ")+" {")
	if s.TLS == "auto" && s.Provider != "" {
		lines = append(lines, "  tls {", "    dns "+s.Provider+" {env."+s.Credential+"}", "  }")
	}
	if s.TLS == "custom" {
		lines = append(lines, "  tls "+s.CertFile+" "+s.KeyFile)
	}
	matched := s.Path != "" || len(a) > 1
	indent := "  "
	if matched {
		lines = append(lines, "  @docklet {")
		if len(a) > 1 {
			lines = append(lines, "    host "+s.Domain)
		}
		if s.Path != "" {
			lines = append(lines, "    path "+s.Path+"*")
		}
		lines = append(lines, "  }", "  handle @docklet {")
		indent = "    "
	}
	lines = append(lines, indent+"route {")
	for _, m := range s.Middlewares {
		lines = append(lines, indent+"  "+middlewareLines[m])
	}
	targets := strings.Join(s.Upstreams, " ")
	if len(s.Upstreams) > 1 {
		lines = append(lines, indent+"  reverse_proxy "+targets+" {", indent+"    lb_policy round_robin", indent+"  }")
	} else {
		lines = append(lines, indent+"  reverse_proxy "+targets)
	}
	lines = append(lines, indent+"}")
	if matched {
		lines = append(lines, "  }")
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n") + "\n", nil
}
func normalized(text string) []string {
	out := []string{}
	for _, l := range strings.Split(text, "\n") {
		if v := strings.TrimSpace(l); v != "" {
			out = append(out, v)
		}
	}
	return out
}
func ParseSite(text string) (Site, error) {
	lines := normalized(text)
	index := 0
	take := func() string {
		if index >= len(lines) {
			index++
			return ""
		}
		v := lines[index]
		index++
		return v
	}
	peek := func() string {
		if index >= len(lines) {
			return ""
		}
		return lines[index]
	}
	s := Site{TLS: "auto", Upstreams: []string{}, Middlewares: []string{}}
	l := take()
	if !strings.HasPrefix(l, "# docklet-route: ") {
		return s, invalidSite()
	}
	s.Name = strings.TrimPrefix(l, "# docklet-route: ")
	if strings.HasPrefix(peek(), "# docklet-issuer: ") {
		s.Resolver = strings.TrimPrefix(take(), "# docklet-issuer: ")
	}
	if strings.HasPrefix(peek(), "# docklet-certificate-domain: ") {
		s.CertificateDomain = strings.TrimPrefix(take(), "# docklet-certificate-domain: ")
	}
	l = take()
	if !strings.HasSuffix(l, " {") {
		return s, invalidSite()
	}
	l = strings.TrimSuffix(l, " {")
	if strings.HasPrefix(l, "http://") {
		s.TLS = "off"
		l = strings.TrimPrefix(l, "http://")
	}
	a := strings.Split(l, ", ")
	s.Domain = a[0]
	if peek() == "tls {" {
		take()
		dns := regexp.MustCompile(`(?i)^dns ([a-z0-9_]+) \{env\.([A-Z_][A-Z0-9_]*)\}$`).FindStringSubmatch(take())
		if dns == nil || s.TLS == "off" {
			return s, invalidSite()
		}
		s.Provider = dns[1]
		s.Credential = dns[2]
		if take() != "}" {
			return s, invalidSite()
		}
	} else if strings.HasPrefix(peek(), "tls ") {
		parts := strings.Split(take(), " ")
		if len(parts) != 3 || s.TLS == "off" {
			return s, invalidSite()
		}
		s.TLS = "custom"
		s.CertFile = parts[1]
		s.KeyFile = parts[2]
	}
	matched := peek() == "@docklet {"
	if matched {
		take()
		if len(a) > 1 && take() != "host "+s.Domain {
			return s, invalidSite()
		}
		if strings.HasPrefix(peek(), "path ") {
			l = take()
			if !strings.HasSuffix(l, "*") {
				return s, invalidSite()
			}
			s.Path = strings.TrimSuffix(strings.TrimPrefix(l, "path "), "*")
		}
		if take() != "}" || take() != "handle @docklet {" {
			return s, invalidSite()
		}
	}
	if take() != "route {" {
		return s, invalidSite()
	}
	// The grammar has only three distinct middleware handlers.
	for attempts := 0; attempts <= len(middlewareLines); attempts++ {
		found := ""
		for m, l := range middlewareLines {
			if peek() == l {
				found = m
			}
		}
		if found == "" {
			break
		}
		s.Middlewares = append(s.Middlewares, found)
		take()
	}
	l = take()
	if !strings.HasPrefix(l, "reverse_proxy ") {
		return s, invalidSite()
	}
	l = strings.TrimPrefix(l, "reverse_proxy ")
	multi := strings.HasSuffix(l, " {")
	l = strings.TrimSuffix(l, " {")
	s.Upstreams = strings.Split(l, " ")
	if multi {
		if take() != "lb_policy round_robin" || take() != "}" {
			return s, invalidSite()
		}
	}
	if multi != (len(s.Upstreams) > 1) || take() != "}" {
		return s, invalidSite()
	}
	if matched && take() != "}" {
		return s, invalidSite()
	}
	if take() != "}" || index != len(lines) {
		return s, invalidSite()
	}
	rendered, err := s.Render()
	if err != nil {
		return s, err
	}
	if !slices.Equal(normalized(rendered), lines) {
		return s, invalidSite()
	}
	return s, nil
}
