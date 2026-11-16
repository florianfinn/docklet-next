// Source: docs/design/proxy-module.md
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original proof that engine diagnostics reach the receipt (S4 dependency gap).

package proxy

import (
	"context"
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

type restoreFailEngine struct {
	*fakeEngine
	calls int
}

func (e *restoreFailEngine) Reload(context.Context, Intent) error {
	e.calls++
	if e.calls == 1 {
		return errors.New("example initial reload failure")
	}
	return refusal("restore reload refused")
}

type firstCleanupFailEngine struct {
	*fakeEngine
	calls int
}

func (e *firstCleanupFailEngine) Cleanup(context.Context, Intent) error {
	e.calls++
	if e.calls == 1 {
		return refusal("recovery cleanup refused")
	}
	return nil
}

// A restore failure and a first recovery cleanup failure keep their diagnostic, also
// when a later cleanup succeeds. Adapted from the main-thread verify probes.
func TestDiagnosticFromRestoreAndRecoveryCleanup(t *testing.T) {
	t.Run("restore", func(t *testing.T) {
		x := newFixture(t)
		x.request.Instance.Kind = dto.Caddy
		x.a.Engines[dto.Caddy] = &restoreFailEngine{fakeEngine: x.e}
		o, err := x.a.Apply(t.Context(), x.request)
		require.NoError(t, err)
		require.Equal(t, dto.RecoveryRequired, o.State)
		require.NotNil(t, o.Diagnostic)
		require.Equal(t, "restore reload refused", o.Diagnostic.Detail)
	})
	t.Run("recovery cleanup", func(t *testing.T) {
		x := newFixture(t)
		x.a.Engines[dto.Traefik] = &firstCleanupFailEngine{fakeEngine: x.e}
		i := Intent{Request: x.request, Old: x.f.files[x.request.Target], New: x.e.new}
		paths(&i)
		o := dto.Operation{ID: x.request.ID, InstanceID: x.request.Instance.ID, Generation: x.request.Generation, Registration: x.request.Registration, State: dto.Pending, Phase: dto.Staging}
		x.persist(t, i, o)
		got, err := x.a.recover(t.Context(), i, o)
		require.NoError(t, err)
		require.Equal(t, dto.RecoveryRequired, got.State)
		require.NotNil(t, got.Diagnostic)
		require.Equal(t, "recovery cleanup refused", got.Diagnostic.Detail)
	})
}

// Release and Query keep an earlier diagnostic on the final receipt.
func TestReleaseKeepsDiagnostic(t *testing.T) {
	x := newFixture(t)
	i := Intent{Request: x.request, Old: x.f.files[x.request.Target], New: x.e.new}
	paths(&i)
	d := &dto.Diagnostic{Path: "routes/app.caddy", Line: 3, Detail: "example initial refusal"}
	x.persist(t, i, dto.Operation{ID: x.request.ID, InstanceID: x.request.Instance.ID, Generation: x.request.Generation, Registration: x.request.Registration, State: dto.RecoveryRequired, Phase: dto.Restoring, Diagnostic: d})
	got, err := x.a.Release(t.Context(), dto.ReleaseRequest{Query: x.query(), Actor: "example-admin", Confirmed: i.Old.FileState})
	require.NoError(t, err)
	require.Equal(t, dto.Released, got.State)
	require.Equal(t, d, got.Diagnostic)
	queried, err := x.a.Query(t.Context(), x.query())
	require.NoError(t, err)
	require.Equal(t, d, queried.Diagnostic)
}

// An eligibility refusal carries the inventory's own text, bounded like a diagnostic.
func TestEligibilityRefusalKeepsInventoryText(t *testing.T) {
	i := dto.Instance{Source: dto.Source{Kind: dto.ProjectSource}, Eligibility: dto.Eligibility{Reason: dto.InvalidGraph, Path: "Caddyfile", Line: 3, Detail: "unrecognized directive: example_directive"}}
	var p *dto.Problem
	require.ErrorAs(t, Eligibility(i), &p)
	require.Equal(t, "invalid-graph: unrecognized directive: example_directive", p.Detail)
	i.Eligibility.Detail = strings.Repeat("x", 2*MaxDiagnostic)
	require.ErrorAs(t, Eligibility(i), &p)
	require.LessOrEqual(t, len(p.Detail), MaxDiagnostic)
}
