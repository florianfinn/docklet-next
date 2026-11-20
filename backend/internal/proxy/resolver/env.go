// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver execution, recovery and verification.

// Package resolver owns secret-bearing memory and the resolver recovery oracle.
package resolver

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/compose-spec/compose-go/v2/dotenv"
	dto "github.com/florianfinn/docklet-next/types/proxy"
)

var envAssignment = regexp.MustCompile(`^(?:export[ \t]+)?([A-Za-z_][A-Za-z0-9_]*)[ \t]*=`)

// Environment appends single-quoted literals. Existing bytes are never reencoded;
// only existence/duplicates and effective literal equality leave the parser.
func Environment(old []byte, vars []dto.ResolverVariable) ([]byte, map[string]string, error) {
	values, err := dotenv.ParseWithLookup(bytes.NewReader(old), func(string) (string, bool) { return "", false })
	if err != nil {
		return nil, nil, dto.Fail(dto.EnvConflict, 422)
	}
	counts := map[string]int{}
	for _, line := range strings.Split(string(old), "\n") {
		if m := envAssignment.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			counts[m[1]]++
		}
	}
	for k := range values {
		if strings.HasPrefix(k, "COMPOSE_") {
			return nil, nil, layout(dto.ComposeEnvSelection)
		}
	}
	out := append([]byte(nil), old...)
	expected := map[string]string{}
	newline := "\n"
	if bytes.Contains(old, []byte("\r\n")) {
		newline = "\r\n"
	}
	final := len(old) == 0 || old[len(old)-1] == '\n'
	for _, v := range vars {
		if counts[v.Name] > 1 {
			return nil, nil, dto.Fail(dto.EnvDuplicate, 422)
		}
		existing, found := values[v.Name]
		if v.Value == nil {
			if !found {
				return nil, nil, dto.Fail(dto.EnvConflict, 422)
			}
			if strings.ContainsAny(existing, "\r\n") {
				return nil, nil, dto.Fail(dto.EnvConflict, 422)
			}
			expected[v.Name] = existing
			continue
		}
		if found {
			return nil, nil, dto.Fail(dto.EnvKeyExists, 422)
		}
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, []byte(newline)...)
		}
		out = append(out, []byte(v.Name+"='"+*v.Value+"'")...)
		if final {
			out = append(out, []byte(newline)...)
		}
		expected[v.Name] = *v.Value
	}
	parsed, err := dotenv.ParseWithLookup(bytes.NewReader(out), func(string) (string, bool) { return "", false })
	if err != nil {
		return nil, nil, dto.Fail(dto.EnvConflict, 422)
	}
	for k, v := range expected {
		if parsed[k] != v {
			return nil, nil, dto.Fail(dto.EnvConflict, 422)
		}
	}
	return out, expected, nil
}

// Redact is for ephemeral diagnostics only. Persistent diagnostics use codes;
// dropping raw parser/compose text also protects reused and inline credentials.
func Redact(text string, vars []dto.ResolverVariable) string {
	for _, v := range vars {
		if v.Value != nil && *v.Value != "" {
			text = strings.ReplaceAll(text, *v.Value, "[redacted]")
		}
	}
	return text
}
