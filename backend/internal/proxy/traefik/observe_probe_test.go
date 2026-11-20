// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

package traefik

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestP7HelperNoRouteExactHash(t *testing.T) {
	original := hostio.Response{Status: 404, BodyHash: "b16e15764b8bc06c5c3f9f19bc8b99fa48e7894aa5a6ccdad65da49bbf564793", Probe: &dto.ProbeEvidence{Client: dto.HelperClient, ImageID: "local-image"}}
	require.True(t, noRoute(original))
	for _, modify := range []func(*hostio.Response){func(r *hostio.Response) { r.BodyHash = "backend-404" }, func(r *hostio.Response) { r.Truncated = true }, func(r *hostio.Response) { r.Followed = true }, func(r *hostio.Response) { r.Status = 302 }, func(r *hostio.Response) { r.Status = 502 }} {
		r := original
		modify(&r)
		require.False(t, noRoute(r), "only the measured complete 404 is no-route")
	}
	backend := original
	backend.BodyHash = "backend-404"
	require.True(t, routingEvidence("create-route", original, backend))
	require.False(t, routingEvidence("create-route", backend, original))
}

type probeWindowRuntime struct {
	runtime
	check func(context.Context)
}

func (r probeWindowRuntime) Probe(ctx context.Context, _ dto.Instance, _, _ string) (hostio.Response, error) {
	r.check(ctx)
	return hostio.Response{Status: 201}, nil
}
func TestP11TraefikProbeUsesWindowDeadline(t *testing.T) {
	x := fixture(t)
	in := x.prepare(x.request("edit-route"))
	checked := false
	x.e.run = probeWindowRuntime{runtime: x.d, check: func(ctx context.Context) {
		checked = true
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), 16*time.Second)
	}}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	_, err := x.e.Observe(ctx, in, false)
	require.NoError(t, err)
	require.True(t, checked)
}
func TestP1PathOnlyProbeAndRouteTLS(t *testing.T) {
	calls := routeProbes(dto.Route{ID: "path", Rule: "PathPrefix(`/api`)", TLS: true}, dto.Inventory{EntryPoints: []dto.EntryPoint{{Name: "secure", Address: ":8443"}}})
	require.Len(t, calls, 1)
	require.Equal(t, "https://127.0.0.1:8443/api", calls[0].URL)
	require.Equal(t, "probe.example.test", calls[0].Host)
}

type certificateRuntime struct {
	runtime
	call func(context.Context, dto.Instance, string, string) (hostio.Response, error)
}

func (r certificateRuntime) Probe(ctx context.Context, i dto.Instance, host, rawURL string) (hostio.Response, error) {
	return r.call(ctx, i, host, rawURL)
}
func TestManualCertificateRereadsRouteAroundProbe(t *testing.T) {
	for _, changed := range []bool{false, true} {
		x := fixture(t)
		x.inspect.Config.Cmd = append(x.inspect.Config.Cmd, "--entrypoints.websecure.http.tls=true")
		x.put("dynamic/routes.yml", strings.Replace(tiny, "[web]", "[websecure]", 1))
		read, err := x.e.Read(t.Context(), x.i)
		require.NoError(t, err)
		var route dto.Route
		for _, r := range read.Routes {
			if r.Name == "app" && r.File == "dynamic/routes.yml" {
				route = r
			}
		}
		require.NotEmpty(t, route.File)
		hash := ""
		for _, file := range read.Files {
			if file.Path == route.File {
				hash = file.Hash
			}
		}
		checked := false
		x.e.run = certificateRuntime{runtime: x.d, call: func(_ context.Context, _ dto.Instance, host, rawURL string) (hostio.Response, error) {
			checked = true
			require.Equal(t, "app.example.com", host)
			require.True(t, strings.HasPrefix(rawURL, "https://"))
			if changed {
				x.put("dynamic/routes.yml", tiny+"# concurrent edit\n")
			}
			return hostio.Response{Status: 200, Certificate: &dto.CertificateObservation{Route: dto.CertificateRoute{SNI: host}, HostnameMatch: true, Validity: dto.CertificateValid}}, nil
		}}
		q := dto.CertificateCheckRequest{File: route.File, Router: route.Name, SourceHash: hash, Host: "app.example.com"}
		observation, err := x.e.CheckCertificate(t.Context(), x.i, q)
		require.True(t, checked)
		if changed {
			require.Equal(t, dto.RouteChanged, reason(err, ""))
		} else {
			require.NoError(t, err)
			require.Equal(t, route.Name, observation.Route.Name)
			require.Equal(t, "websecure", observation.Route.EntryPoint)
			require.False(t, observation.ChainTrusted)
		}
	}
}
func TestRouteCertificateObservationDoesNotChangeRoutingVerdict(t *testing.T) {
	x := fixture(t)
	in := x.prepare(x.request("edit-route"))
	x.e.run = certificateRuntime{runtime: x.d, call: func(context.Context, dto.Instance, string, string) (hostio.Response, error) {
		return hostio.Response{Status: 201, Certificate: &dto.CertificateObservation{ChainTrusted: false, HostnameMatch: true, Validity: dto.CertificateExpired}, Probe: &dto.ProbeEvidence{Client: dto.HelperClient, ImageID: "sha256:agent-local"}}, nil
	}}
	observation, err := x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, observation.State)
	require.NotNil(t, observation.Certificate)
	require.False(t, observation.Certificate.ChainTrusted)
}

type probeAvailabilityTransport struct {
	base      http.RoundTripper
	available *bool
}

func (r probeAvailabilityTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body := ""
	status := 200
	switch {
	case strings.Contains(request.URL.Path, "/images/") && strings.HasSuffix(request.URL.Path, "/json"):
		if *r.available {
			body = `{"Id":"sha256:replacement"}`
		} else {
			status = 404
			body = `{"message":"not found"}`
		}
	case strings.HasSuffix(request.URL.Path, "/images/create"):
		status = 404
		body = `{"message":"not found"}`
	case strings.HasSuffix(request.URL.Path, "/containers/create"):
		status = 500
		body = `{"message":"exec: /usr/bin/docklet-probe: no such file or directory"}`
	default:
		return r.base.RoundTrip(request)
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}
func TestP4ManualPrewarmCannotReplaceActiveRouteClient(t *testing.T) {
	x := fixture(t)
	original, err := x.d.Docker.GetClient(t.Context())
	require.NoError(t, err)
	available := false
	cli, err := client.New(client.WithHost(original.DaemonHost()), client.WithAPIVersion(original.ClientVersion()), client.WithHTTPClient(&http.Client{Transport: probeAvailabilityTransport{base: http.DefaultTransport, available: &available}}))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cli.Close()) })
	x.d.Docker = x.d.Docker.WithClient(cli)
	route := hostio.WithProbeOperation(t.Context(), "route-operation")
	manual := hostio.WithProbeOperation(t.Context(), "manual-check")
	require.NoError(t, x.d.PrewarmProbe(route, x.i))
	available = true
	require.NoError(t, x.d.PrewarmProbe(manual, x.i))
	before, err := x.d.Probe(route, x.i, "app.example.test", "http://127.0.0.1/")
	require.NoError(t, err)
	require.Equal(t, dto.ProbeImageUnavailable, before.Probe.Reason, "a manual check must not replace the active route's prepared client")
	after, err := x.d.Probe(manual, x.i, "app.example.test", "http://127.0.0.1/")
	require.NoError(t, err)
	require.Equal(t, dto.ProbeClientMissing, after.Probe.Reason)
}
