// Source: docs/design/proxy-module.md
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original proof that engine diagnostics reach the receipt (S4 dependency gap).

package proxy

import (
	"errors"
	"strings"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func refusal(detail string) *dto.Problem {
	p := dto.Fail(dto.InvalidGraph, 422)
	p.Path, p.Line, p.Detail = "routes/app.caddy", 3, detail
	return p
}

// A Prepare refusal and a Validate failure keep the engine's path, line and text in the
// journal; a query after the request returns the same diagnostic.
func TestDiagnosticReachesTheReceipt(t *testing.T) {
	for _, where := range []string{"prepare", "validate", "reload"} {
		t.Run(where, func(t *testing.T) {
			x := newFixture(t)
			p := refusal("Error: adapting config: unrecognized directive: example_directive")
			switch where {
			case "prepare":
				x.e.prepareErr = p
			case "validate":
				x.e.validate = func() error { return p }
			default:
				x.request.Instance.Kind = dto.Caddy
				x.e.reloadErr = p
			}
			o, err := x.a.Apply(t.Context(), x.request)
			require.NoError(t, err)
			require.NotNil(t, o.Diagnostic)
			require.Equal(t, dto.Diagnostic{Path: p.Path, Line: p.Line, Detail: p.Detail}, *o.Diagnostic)
			got, err := x.a.Query(t.Context(), x.query())
			require.NoError(t, err)
			require.Equal(t, o.Diagnostic, got.Diagnostic)
		})
	}
}

// Untyped errors and refusals without location or text carry no diagnostic.
func TestDiagnosticOnlyFromTypedText(t *testing.T) {
	require.Nil(t, diagnostic(errors.New("example plain failure")))
	require.Nil(t, diagnostic(dto.Fail(dto.InvalidGraph, 422)))
	require.NotNil(t, diagnostic(refusal("")))
}

// Stored text is bounded on a rune boundary and keeps no control characters but newline and tab.
func TestDiagnosticIsBounded(t *testing.T) {
	long := strings.Repeat("ä", MaxDiagnostic)
	d := diagnostic(refusal("line one\n\tline two\x1b[31m\r" + long))
	require.LessOrEqual(t, len(d.Detail), MaxDiagnostic)
	require.True(t, strings.HasPrefix(d.Detail, "line one\n\tline two[31m"))
	require.NotContains(t, d.Detail, "\x1b")
	require.NotContains(t, d.Detail, "\r")
	require.True(t, strings.HasSuffix(d.Detail, "ä"), "cut on a rune boundary")
	over := refusal("x")
	over.Path = strings.Repeat("a", 1025)
	require.Empty(t, diagnostic(over).Path)
	negative := refusal("x")
	negative.Line = -1
	require.Zero(t, diagnostic(negative).Line)
}

// An eligibility refusal names the eligibility path and line.
func TestEligibilityRefusalKeepsLocation(t *testing.T) {
	i := dto.Instance{Eligibility: dto.Eligibility{Reason: dto.ImportMissing, Path: "Caddyfile", Line: 7}, Source: dto.Source{Kind: dto.ProjectSource}}
	var p *dto.Problem
	require.ErrorAs(t, Eligibility(i), &p)
	require.Equal(t, dto.SourceReadOnly, p.Code)
	require.Equal(t, string(dto.ImportMissing), p.Detail)
	require.Equal(t, "Caddyfile", p.Path)
	require.Equal(t, 7, p.Line)
}
