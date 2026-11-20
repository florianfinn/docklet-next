// Source: docklet-next/issues/492
// Revision: 551c24a4e4173b0e6eaa00e3ca125cca0a619124
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver summaries for the Traefik inventory.

package traefik

import (
	"sort"
	"strconv"
	"strings"

	dto "github.com/florianfinn/docklet-next/types/proxy"
)

// resolverSummaries describes each configured ACME resolver from the selected
// static source. Only names, CA, storage, challenge and, for curated DNS
// providers, whether each provider variable is set in the container are
// returned; the email and every variable value stay out. A resolver that does
// not fit the summary contract stays in Resolvers only.
func resolverSummaries(selected map[string]string, env []string) []dto.ResolverSummary {
	set := map[string]bool{}
	for _, s := range env {
		if k, v, ok := strings.Cut(s, "="); ok && v != "" {
			set[k] = true
		}
	}
	keys := map[string]map[string]string{}
	for k, v := range selected {
		bits := strings.Split(k, ".")
		if len(bits) < 3 || bits[0] != "certificatesresolvers" {
			continue
		}
		if keys[bits[1]] == nil {
			keys[bits[1]] = map[string]string{}
		}
		keys[bits[1]][strings.ToLower(strings.Join(bits[2:], "."))] = v
	}
	out := []dto.ResolverSummary{}
	for name, k := range keys {
		s, ok := summary(name, k, set)
		if ok {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

func summary(name string, k map[string]string, set map[string]bool) (dto.ResolverSummary, bool) {
	s := dto.ResolverSummary{Name: name, Storage: k["acme.storage"], Variables: []dto.ResolverVariableSummary{}}
	switch url := k["acme.caserver"]; url {
	case "", "https://acme-v02.api.letsencrypt.org/directory":
		s.CA = dto.ResolverCA{Kind: "letsencrypt"}
	case "https://acme-staging-v02.api.letsencrypt.org/directory":
		s.CA = dto.ResolverCA{Kind: "letsencrypt-staging"}
	default:
		s.CA = dto.ResolverCA{Kind: "custom", URL: url}
	}
	if s.Storage == "" || len(s.Storage) > 256 || len(s.CA.URL) > 512 {
		return s, false
	}
	if ep := k["acme.httpchallenge.entrypoint"]; ep != "" {
		s.Challenge = dto.ResolverChallenge{Kind: "http", EntryPoint: ep}
		return s, len(ep) <= 128
	}
	provider := dto.DNSProvider(k["acme.dnschallenge.provider"])
	var definition *dto.DNSProviderDefinition
	for _, d := range dto.DNSProviders() {
		if d.Provider == provider {
			definition = &d
			break
		}
	}
	if definition == nil {
		return s, false
	}
	s.Challenge = dto.ResolverChallenge{Kind: "dns", Provider: provider}
	if v := k["acme.dnschallenge.delaybeforecheck"]; v != "" {
		n, err := strconv.Atoi(strings.TrimSuffix(v, "s"))
		if err != nil || n < 0 || n > 600 {
			return s, false
		}
		s.Challenge.DelayBeforeCheck = n
	}
	for _, v := range definition.Variables {
		s.Variables = append(s.Variables, dto.ResolverVariableSummary{Name: v.Name, Configured: set[v.Name]})
	}
	return s, true
}
