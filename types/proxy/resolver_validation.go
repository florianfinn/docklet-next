// Source: docklet-next/issues/492
// Revision: 52bae4d0610f04e66451664e2c0741699c901873
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original pure resolver ingress validation, without host effects.

package proxy

import (
	"net"
	"net/mail"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var resolverName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,62}$`)
var variableName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
var entryPointName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
var storagePath = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
var resolverHost = regexp.MustCompile(`^[A-Za-z0-9.:-]+$`)

// unsafeLiteral rejects characters that YAML, Compose interpolation or the
// command list would reinterpret; values are written verbatim by the executor.
func unsafeLiteral(s string) bool {
	for _, c := range s {
		if c < 0x21 || c == 0x7f || strings.ContainsRune("$'\"`\\", c) {
			return true
		}
	}
	return false
}

// Validate checks live input before a stub or future executor can admit work.
// Inventory uniqueness, storage mounts and effective Compose checks belong to admission.
func (q ResolverRequest) Validate() error {
	invalid := func() error { return Fail(InvalidInput, 422) }
	if !resolverName.MatchString(q.Name) || len(q.Email) > 254 || len(q.Email) == 0 || unsafeLiteral(q.Email) {
		return invalid()
	}
	email, err := mail.ParseAddress(q.Email)
	if err != nil || email.Address != q.Email {
		return invalid()
	}
	switch q.CA.Kind {
	case "letsencrypt", "letsencrypt-staging":
		if q.CA.URL != "" {
			return invalid()
		}
	case "custom":
		u, err := url.Parse(q.CA.URL)
		if err != nil || len(q.CA.URL) > 512 || unsafeLiteral(q.CA.URL) || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return invalid()
		}
	default:
		return invalid()
	}
	if len(q.Storage) == 0 || len(q.Storage) > 256 || !path.IsAbs(q.Storage) || path.Clean(q.Storage) != q.Storage || !storagePath.MatchString(q.Storage) {
		return invalid()
	}
	c := q.Challenge
	if len(q.Variables) > 16 || c.DelayBeforeCheck < 0 || c.DelayBeforeCheck > 600 || len(c.Resolvers) > 4 {
		return invalid()
	}
	switch c.Kind {
	case "http":
		if !entryPointName.MatchString(c.EntryPoint) || c.Provider != "" || c.DelayBeforeCheck != 0 || len(c.Resolvers) != 0 || len(q.Variables) != 0 {
			return invalid()
		}
	case "dns":
		if c.EntryPoint != "" {
			return invalid()
		}
		if err := validateProviderVariables(c.Provider, q.Variables); err != nil {
			return err
		}
	default:
		return invalid()
	}
	for _, resolver := range c.Resolvers {
		host, port, err := net.SplitHostPort(resolver)
		n, er := strconv.Atoi(port)
		if err != nil || er != nil || !resolverHost.MatchString(host) || n < 1 || n > 65535 {
			return invalid()
		}
	}
	return nil
}
func validateProviderVariables(provider DNSProvider, variables []ResolverVariable) error {
	var definition *DNSProviderDefinition
	for _, d := range DNSProviders() {
		if d.Provider == provider {
			definition = &d
			break
		}
	}
	if definition == nil {
		return Fail(InvalidInput, 422)
	}
	names := map[string]string{}
	allowed := map[string]DNSProviderVariable{}
	for _, v := range definition.Variables {
		allowed[v.Name] = v
	}
	canonical := map[string]bool{}
	for _, v := range variables {
		entry, ok := allowed[v.Name]
		if !ok || !variableName.MatchString(v.Name) || strings.HasPrefix(v.Name, "COMPOSE_") || strings.HasPrefix(v.Name, "DOCKER_") || v.Name == "PATH" || v.Name == "HOME" {
			return Fail(InvalidInput, 422)
		}
		if _, exists := names[v.Name]; exists {
			return Fail(InvalidInput, 422)
		}
		names[v.Name] = v.Name
		name := v.Name
		if entry.AliasOf != "" {
			name = entry.AliasOf
		}
		// Two spellings of one credential (an alias and its canonical name) are refused.
		if canonical[name] {
			return Fail(InvalidInput, 422)
		}
		canonical[name] = true
		if v.Value != nil && (len(*v.Value) == 0 || len(*v.Value) > 4096 || strings.ContainsAny(*v.Value, "\x00\r\n'")) {
			return Fail(InvalidInput, 422)
		}
	}
	for _, set := range definition.CredentialSets {
		satisfied := true
		for _, name := range set {
			satisfied = satisfied && canonical[name]
		}
		if satisfied {
			return nil
		}
	}
	return Fail(InvalidInput, 422)
}

// Validate uses only names and modes; it never reconstructs credential values.
func (i ResolverIntent) Validate() error {
	q := ResolverRequest{ResolverConfig: i.ResolverConfig, Variables: make([]ResolverVariable, 0, len(i.Variables))}
	for _, v := range i.Variables {
		if v.Mode != "new" && v.Mode != "reuse" {
			return Fail(InvalidInput, 422)
		}
		q.Variables = append(q.Variables, ResolverVariable{Name: v.Name})
	}
	return q.Validate()
}
