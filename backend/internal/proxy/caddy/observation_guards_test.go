// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original deadline, first-response and semantic readback counterexamples.

package caddy

import (
	"context"
	"encoding/json/v2"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestRoutingDecisionTable(t *testing.T) {
	for _, tt := range []struct {
		kind          string
		before, after int
		want          bool
	}{
		{"edit-route", 200, 200, false}, {"edit-route", 200, 0, false}, {"edit-route", 200, 401, true}, {"create-route", 0, 200, true}, {"create-route", 200, 201, true}, {"create-route", 404, 0, false}, {"create-route", 404, 404, false}, {"create-route", 404, 401, true}, {"delete-route", 0, 404, false}, {"delete-route", 404, 0, true}, {"delete-route", 200, 0, true}, {"delete-route", 200, 404, true}, {"delete-route", 200, 201, true}, {"delete-route", 200, 200, false}, {"delete-route", 200, 502, false},
	} {
		require.Equal(t, tt.want, routing(hostio.Response{Status: tt.before}, hostio.Response{Status: tt.after}, tt.kind), "%+v", tt)
	}
}

// Caddy 2.10.2 answers a host without a matching handler with an empty 200 (baseline B0,
// sandbox measurement 2026-11-15); a catch-all site answers with its own response.
const measuredUnmatched = "Connecting to 127.0.0.1 (127.0.0.1:80)\n  HTTP/1.1 200 OK\n  Server: Caddy\n  Date: Sun, 15 Nov 2026 00:37:54 GMT\n  Content-Length: 0\n  Connection: close\n  \nwriting to stdout\nwritten to stdout\n"

func TestRoutingWithMeasuredUnmatchedAndCatchAll(t *testing.T) {
	unmatched, err := hostio.ParseProbe(hostio.ExecResult{Stderr: []byte(measuredUnmatched)}, nil)
	require.NoError(t, err)
	site, err := hostio.ParseProbe(hostio.ExecResult{Stdout: []byte("Hostname: example-whoami-b"), Stderr: []byte("  HTTP/1.1 200 OK\n  Content-Type: text/plain; charset=utf-8\n  Content-Length: 25\n  \n")}, nil)
	require.NoError(t, err)
	catchAll, err := hostio.ParseProbe(hostio.ExecResult{Stdout: []byte("Hostname: example-whoami-a"), Stderr: []byte("  HTTP/1.1 200 OK\n  Content-Type: text/plain; charset=utf-8\n  Content-Length: 25\n  \n")}, nil)
	require.NoError(t, err)
	require.True(t, routing(unmatched, site, "create-route"), "a new site replaces the empty 200")
	require.True(t, routing(catchAll, site, "create-route"), "a new site replaces the catch-all answer")
	require.False(t, routing(catchAll, catchAll, "create-route"), "the same answer is no evidence")
	require.True(t, routing(site, unmatched, "delete-route"))
	require.True(t, routing(site, catchAll, "delete-route"))
	require.False(t, routing(site, hostio.Response{Status: 502}, "delete-route"))
}
func observationIntent(x *dockerFixture, s Site, adminOff bool, kind string) proxy.Intent {
	b, _ := json.Marshal(state{Root: "/etc/caddy/Caddyfile", Admin: "localhost:2019", AdminOff: adminOff, Before: hostio.Response{Status: 200, Headers: map[string]string{"server": "Caddy"}}, NewSite: &s})
	return proxy.Intent{Request: dto.ApplyRequest{Instance: x.i, Change: dto.Change{Kind: kind}}, Engine: b}
}
func TestP6NoProofAfterDeadlineEvenWhenResponseChanges(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(map[bool]string{false: "after-wait", true: "during-probe"}[late], func(t *testing.T) {
			x := fixture(t)
			s := baseSite()
			i := observationIntent(x, s, true, "edit-route")
			calls := 0
			x.execFn = func([]string) hostio.ExecResult {
				calls++
				if late {
					x.now = x.now.Add(6 * time.Second)
					return httpResult(201, "late")
				}
				if x.waits > 0 {
					return httpResult(201, "late")
				}
				return httpResult(200, "")
			}
			x.e.wait = func(context.Context, time.Duration) error {
				x.mu.Lock()
				defer x.mu.Unlock()
				x.waits++
				x.now = x.now.Add(5 * time.Second)
				return nil
			}
			o, err := x.e.Observe(t.Context(), i, false)
			require.NoError(t, err)
			require.Equal(t, dto.AppliedUnverified, o.State)
			require.Equal(t, dto.NoObservableDifference, o.Reason)
			require.Equal(t, 1, calls)
		})
	}
}
func TestInitialEpochAndWaitFailuresCannotVerify(t *testing.T) {
	x := fixture(t)
	i := observationIntent(x, baseSite(), true, "edit-route")
	x.c.State.StartedAt = nextEpoch
	o, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.ContainerChanged, o.Reason)
	require.Empty(t, x.commands)
	x.c.State.StartedAt = epoch
	x.execFn = func([]string) hostio.ExecResult { return httpResult(200, "") }
	x.e.wait = func(context.Context, time.Duration) error {
		x.now = x.now.Add(6 * time.Second)
		return context.Canceled
	}
	// A failed wait inside a live operation ends the window: never verified, never uncertain.
	o, err = x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
}
func TestAdminOffUnobservableAndAdminOnMismatchCannotUseConfigurationEvidence(t *testing.T) {
	for _, adminOff := range []bool{false, true} {
		t.Run(map[bool]string{false: "admin-on", true: "admin-off"}[adminOff], func(t *testing.T) {
			x := fixture(t)
			s := baseSite()
			s.TLS = "auto"
			i := observationIntent(x, s, adminOff, "edit-route")
			x.execFn = func(args []string) hostio.ExecResult {
				if strings.Contains(args[len(args)-1], "/config/") {
					return httpResult(200, "{}")
				}
				return hostio.ExecResult{ExitCode: 1}
			}
			x.e.wait = func(context.Context, time.Duration) error { x.now = x.now.Add(5 * time.Second); return nil }
			o, err := x.e.Observe(t.Context(), i, false)
			require.NoError(t, err)
			require.Equal(t, dto.AppliedUnverified, o.State)
			require.Equal(t, dto.NoObservableDifference, o.Reason)
		})
	}
}
func TestHTTPUnchangedReadbackReportsNoObservableDifference(t *testing.T) {
	x := fixture(t)
	s := baseSite()
	i := observationIntent(x, s, false, "edit-route")
	x.execFn = func(args []string) hostio.ExecResult {
		if strings.Contains(args[len(args)-1], "/config/") {
			return httpResult(200, configFor(s))
		}
		return httpResult(200, "")
	}
	x.e.wait = func(context.Context, time.Duration) error { x.now = x.now.Add(5 * time.Second); return nil }
	o, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
	require.Equal(t, dto.NoObservableDifference, o.Reason)
}
func TestAdminReadbackRecoveryClearsOnlyAfterMatchingConfiguration(t *testing.T) {
	for _, matching := range []bool{false, true} {
		t.Run(map[bool]string{false: "mismatch", true: "match"}[matching], func(t *testing.T) {
			x := fixture(t)
			s := baseSite()
			i := observationIntent(x, s, false, "edit-route")
			readbacks := 0
			x.execFn = func(args []string) hostio.ExecResult {
				if strings.Contains(args[len(args)-1], "/config/") {
					readbacks++
					if readbacks == 1 {
						return hostio.ExecResult{ExitCode: 1}
					}
					if matching {
						return httpResult(200, configFor(s))
					}
					return httpResult(200, "{}")
				}
				return httpResult(200, "")
			}
			x.e.wait = func(context.Context, time.Duration) error { x.now = x.now.Add(2500 * time.Millisecond); return nil }
			o, err := x.e.Observe(t.Context(), i, false)
			require.NoError(t, err)
			require.Equal(t, dto.AppliedUnverified, o.State)
			want := dto.AdminUnavailable
			if matching {
				want = dto.NoObservableDifference
			}
			require.Equal(t, want, o.Reason)
		})
	}
}
func TestP10AdminOffDeleteUsesTheOldRouteProbeAndNewEpoch(t *testing.T) {
	x := fixture(t)
	x.put("Caddyfile", strings.Replace(rootText, "admin localhost:2019", "admin off", 1))
	x.serving()
	s := baseSite()
	q := x.request("create-route", s)
	q.Acknowledgements.Restart = true
	o, err := applyChecked(t, x.a, q)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
	q = x.request("delete-route", s)
	q.Acknowledgements.Restart = true
	o, err = applyChecked(t, x.a, q)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
	require.Equal(t, dto.Routing, o.Evidence)
	require.Equal(t, 2, x.restarts)
	x.noTransient()
}
func TestReadbackMatchesInheritedRoutesAndDeletionIgnoresOldUpstreams(t *testing.T) {
	s := baseSite()
	var config map[string]any
	require.NoError(t, json.Unmarshal([]byte(configFor(s)), &config))
	servers := object(object(object(config["apps"])["http"])["servers"])
	route := object(array(object(servers["example"])["routes"])[0])
	handler := array(route["handle"])[0]
	route["handle"] = []any{map[string]any{"handler": "subroute", "routes": []any{map[string]any{"handle": []any{handler}}}}}
	require.True(t, containsSite(config, s, true))
	changed := s
	changed.Upstreams = []string{"http://192.0.2.90:8080"}
	require.True(t, containsSite(config, changed, false))
	require.False(t, containsSite(config, changed, true))
	var got bool
	require.NotPanics(t, func() { got = upstreamsEqual(map[string]any{}, []string{"http://["}) })
	require.False(t, got)
}
