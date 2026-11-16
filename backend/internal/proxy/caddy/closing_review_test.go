// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing review counterexamples (C-B1 producer forms, C-B2, C-B3, C-B4).

package caddy

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestClosingReviewerProducerCaddyForms(t *testing.T) {
	for _, form := range []string{"CRLF", "duplicate", "CRLF-matched"} {
		t.Run(form, func(t *testing.T) {
			x := fixture(t)
			s := baseSite()
			s.Name = "app"
			s.Upstreams = []string{"http://backend.example.com:8080"}
			if form == "duplicate" {
				s.Upstreams = append(s.Upstreams, s.Upstreams[0])
			}
			if form == "CRLF-matched" {
				s.TLS = "auto"
				s.CertificateDomain = "example.com"
				s.Provider = "cloudflare"
				s.Credential = "EXAMPLE_DNS_TOKEN"
			}
			text, err := s.Render()
			require.NoError(t, err)
			if strings.HasPrefix(form, "CRLF") {
				text = strings.ReplaceAll(text, "\n", "\r\n")
			}
			parsed, err := ParseSite(text)
			require.NoError(t, err)
			require.Equal(t, s, parsed)
			x.owned("routes/app.caddy", text)
			read, err := x.e.Read(t.Context(), x.i)
			require.NoError(t, err)
			found := false
			for _, r := range read.Routes {
				if r.ID == "routes/app.caddy" {
					found = true
					require.Equal(t, "editable", r.Class)
					require.True(t, r.Eligibility.Writable)
					require.Equal(t, s.Upstreams, r.Upstreams)
				}
			}
			require.True(t, found)
		})
	}
}

func TestClosingReviewerMissingBeforeIsNotRoutingEvidence(t *testing.T) {
	x := fixture(t)
	s := baseSite()
	x.put("Caddyfile", strings.ReplaceAll(rootText, "admin localhost:2019", "admin off"))
	text, err := s.Render()
	require.NoError(t, err)
	x.owned("routes/app.caddy", text)
	next := s
	next.Name = "renamed"
	q := x.request("edit-route", next)
	q.Acknowledgements.Restart = true
	creates := 0
	probes := 0
	x.fault = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/exec") {
			creates++
			if creates == 1 {
				http.Error(w, `{"message":"daemon temporarily busy"}`, http.StatusConflict)
				return true
			}
		}
		return false
	}
	x.execFn = func([]string) hostio.ExecResult { probes++; return httpResult(200, "") }
	i, err := x.e.Prepare(t.Context(), q)
	require.NoError(t, err)
	st, err := decode(i)
	require.NoError(t, err)
	require.Equal(t, dto.NotObservable, st.ProbeReason)
	x.e.wait = func(context.Context, time.Duration) error { x.now = x.now.Add(5 * time.Second); return nil }
	o, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Positive(t, probes)
	require.NotEqual(t, dto.Verified, o.State, "a failed Docker exec is not an observed before-response")
}

func TestClosingReviewerTransientEpochIsNotContainerChange(t *testing.T) {
	for _, stage := range []string{"initial", "settlement"} {
		t.Run(stage, func(t *testing.T) {
			x := fixture(t)
			s := baseSite()
			i := observationIntent(x, s, true, "edit-route")
			ready := stage == "initial"
			x.execFn = func([]string) hostio.ExecResult { ready = true; return httpResult(201, "changed") }
			failed := false
			x.fault = func(w http.ResponseWriter, r *http.Request) bool {
				if ready && !failed && strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json") {
					failed = true
					http.Error(w, `{"message":"daemon temporarily busy"}`, http.StatusServiceUnavailable)
					return true
				}
				return false
			}
			o, err := x.e.Observe(t.Context(), i, false)
			require.True(t, failed)
			require.NoError(t, err)
			require.NotEqual(t, dto.ContainerChanged, o.Reason, "an inspect transport failure does not prove a new identity")
		})
	}
}

func TestClosingReviewerMissingBeforeRealApply(t *testing.T) {
	x := fixture(t)
	s := baseSite()
	x.put("Caddyfile", strings.ReplaceAll(rootText, "admin localhost:2019", "admin off"))
	text, err := s.Render()
	require.NoError(t, err)
	x.owned("routes/app.caddy", text)
	next := s
	next.Name = "renamed"
	q := x.request("edit-route", next)
	q.Acknowledgements.Restart = true
	creates := 0
	x.fault = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/exec") {
			creates++
			if creates == 1 {
				http.Error(w, `{"message":"daemon temporarily busy"}`, http.StatusConflict)
				return true
			}
		}
		return false
	}
	x.execFn = func([]string) hostio.ExecResult { return httpResult(200, "") }
	o, err := x.a.Apply(t.Context(), q)
	require.NoError(t, err)
	require.NotEqual(t, dto.Verified, o.State, "a pure rename with a failed before-exec cannot claim a routing difference")
}
func TestClosingReviewerLogsDeadlineUsesLastReason(t *testing.T) {
	x := fixture(t)
	i := observationIntent(x, baseSite(), true, "edit-route")
	x.e.activated.Store(i.Request.ID, x.now.Add(-4800*time.Millisecond))
	x.fault = func(_ http.ResponseWriter, r *http.Request) bool {
		if strings.Contains(r.URL.Path, "/logs") {
			<-r.Context().Done()
			return true
		}
		return false
	}
	o, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.NoObservableDifference, o.Reason, "window expiry during the log read retains the last reason")
}
