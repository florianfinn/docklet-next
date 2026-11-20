// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

package caddy

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

type probeWindowRuntime struct {
	Runtime
	check func(context.Context)
}

func (r probeWindowRuntime) Probe(ctx context.Context, _ dto.Instance, _, _ string) (hostio.Response, error) {
	r.check(ctx)
	return hostio.Response{Status: 201}, nil
}
func TestP11CaddyProbeUsesWindowDeadline(t *testing.T) {
	x := fixture(t)
	in := observationIntent(x, baseSite(), true, "edit-route")
	checked := false
	x.e.runtime = probeWindowRuntime{Runtime: x.d, check: func(ctx context.Context) {
		checked = true
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), 7*time.Second)
	}}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	_, err := x.e.Observe(ctx, in, false)
	require.NoError(t, err)
	require.True(t, checked)
}

type certificateRuntime struct {
	Runtime
	call func(context.Context, dto.Instance, string, string) (hostio.Response, error)
}

func (r certificateRuntime) Probe(ctx context.Context, i dto.Instance, host, rawURL string) (hostio.Response, error) {
	return r.call(ctx, i, host, rawURL)
}
func TestManualCertificateUsesCaddySitePathAndSourceFence(t *testing.T) {
	for _, changed := range []bool{false, true} {
		x := fixture(t)
		site := baseSite()
		site.TLS = "auto"
		site.Path = "/api"
		text, err := site.Render()
		require.NoError(t, err)
		x.put("routes/app.caddy", text)
		read, err := x.e.Read(t.Context(), x.i)
		require.NoError(t, err)
		hash := ""
		for _, file := range read.Files {
			if file.Path == "routes/app.caddy" {
				hash = file.Hash
			}
		}
		checked := false
		x.e.runtime = certificateRuntime{Runtime: x.d, call: func(_ context.Context, _ dto.Instance, host, rawURL string) (hostio.Response, error) {
			checked = true
			require.Equal(t, site.Domain, host)
			require.Equal(t, "https://127.0.0.1/api", rawURL)
			if changed {
				x.put("routes/app.caddy", strings.Replace(text, "/api", "/next", 1))
			}
			return hostio.Response{Status: 401, Certificate: &dto.CertificateObservation{Route: dto.CertificateRoute{SNI: host}, HostnameMatch: true, Validity: dto.CertificateValid}}, nil
		}}
		observation, err := x.e.CheckCertificate(t.Context(), x.i, dto.CertificateCheckRequest{File: "routes/app.caddy", Router: site.Name, SourceHash: hash, Host: site.Domain})
		require.True(t, checked)
		if changed {
			require.Equal(t, dto.RouteChanged, reason(err, ""))
		} else {
			require.NoError(t, err)
			require.Equal(t, site.Name, observation.Route.Name)
			require.False(t, observation.ChainTrusted)
		}
	}
}
