// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original rename-evidence and window-expiry tests from the M27 early smoke 4.

package caddy

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestRenameOnlyComparesEverythingButTheName(t *testing.T) {
	old := baseSite()
	renamed := old
	renamed.Name = "example-renamed"
	require.True(t, renameOnly(old, renamed))
	for name, change := range map[string]func(*Site){
		"domain":    func(s *Site) { s.Domain = "next.example.com" },
		"path":      func(s *Site) { s.Path = "/api" },
		"tls":       func(s *Site) { s.TLS = "internal" },
		"upstream":  func(s *Site) { s.Upstreams = []string{"http://192.0.2.81:8080"} },
		"middlware": func(s *Site) { s.Middlewares = []string{"compress"} },
	} {
		next := renamed
		next.Upstreams = append([]string(nil), renamed.Upstreams...)
		change(&next)
		require.False(t, renameOnly(old, next), name)
	}
}

// Early smoke 4: a pure rename leaves the routing answer unchanged, so the admin
// readback of the identical site is its evidence; without the flag it stays unverified.
func TestPureRenameVerifiesByConfigurationReadback(t *testing.T) {
	for _, rename := range []bool{true, false} {
		x := fixture(t)
		s := baseSite()
		i := observationIntent(x, s, false, "edit-route")
		var st state
		require.NoError(t, json.Unmarshal(i.Engine, &st))
		st.Rename = rename
		i.Engine, _ = json.Marshal(st)
		x.execFn = func(args []string) hostio.ExecResult {
			if strings.Contains(args[len(args)-1], "/config/") {
				return httpResult(200, configFor(s))
			}
			return httpResult(200, "")
		}
		x.e.wait = func(context.Context, time.Duration) error { x.now = x.now.Add(5 * time.Second); return nil }
		o, err := x.e.Observe(t.Context(), i, false)
		require.NoError(t, err)
		if rename {
			require.Equal(t, proxy.Observation{State: dto.Verified, Evidence: dto.Configuration, Probe: &dto.ProbeEvidence{Client: dto.WgetClient, Reason: dto.ProbeImageUnavailable}}, o)
		} else {
			require.Equal(t, dto.NoObservableDifference, o.Reason)
		}
	}
}

func TestPrepareMarksOnlyANameChangeAsRename(t *testing.T) {
	for _, upstream := range []string{"http://192.0.2.80:8080", "http://192.0.2.81:8080"} {
		x := fixture(t)
		s := baseSite()
		text, err := renderChecked(t, s)
		require.NoError(t, err)
		x.owned("routes/app.caddy", text)
		next := s
		next.Name, next.Upstreams = "example-renamed", []string{upstream}
		i, err := x.e.Prepare(t.Context(), x.request("edit-route", next))
		require.NoError(t, err)
		st, err := decode(i)
		require.NoError(t, err)
		require.Equal(t, upstream == s.Upstreams[0], st.Rename, upstream)
	}
}

// Early smoke 4 (host D): an Epoch error caused by the expiring observation window
// is not a container change; only a real identity difference reports one.
func TestEpochErrorAtWindowEndIsNotContainerChanged(t *testing.T) {
	x := fixture(t)
	s := baseSite()
	i := observationIntent(x, s, false, "edit-route")
	x.e.activated.Store(i.Request.ID, x.now.Add(-6800*time.Millisecond))
	slow := false
	x.execFn = func(args []string) hostio.ExecResult {
		if strings.Contains(args[len(args)-1], "/config/") {
			slow = true
			return httpResult(200, configFor(s))
		}
		return httpResult(200, "")
	}
	x.fault = func(_ http.ResponseWriter, r *http.Request) bool {
		if slow && strings.HasPrefix(r.URL.Path, "/v") && strings.Contains(r.URL.Path, "/containers/") {
			time.Sleep(400 * time.Millisecond)
		}
		return false
	}
	o, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
	require.NotEqual(t, dto.ContainerChanged, o.Reason)
}

// Candidate run F3: one failed probe early in an admin-off window must not decide the
// final reason when later probes in the window answer unchanged.
func TestLaterProbeClearsAnEarlierProbeFailureReason(t *testing.T) {
	x := fixture(t)
	i := observationIntent(x, baseSite(), true, "edit-route")
	probes, creates := 0, 0
	x.execFn = func([]string) hostio.ExecResult { probes++; return httpResult(200, "") }
	x.fault = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/exec") {
			if creates++; creates == 1 {
				http.Error(w, `{"message":"container is restarting"}`, http.StatusConflict)
				return true
			}
		}
		return false
	}
	x.e.wait = func(context.Context, time.Duration) error {
		if probes >= 2 {
			x.now = x.now.Add(5 * time.Second)
		}
		return nil
	}
	o, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.GreaterOrEqual(t, creates, 1)
	require.GreaterOrEqual(t, probes, 2)
	require.Equal(t, proxy.Observation{State: dto.AppliedUnverified, Reason: dto.NoObservableDifference, Probe: &dto.ProbeEvidence{Client: dto.WgetClient, Reason: dto.ProbeImageUnavailable}}, o)
}

// Candidate delta 1: a probe or readback cut off by the end of the observation window is
// not the operation's reason (Traefik already maps this to no-observable-difference).
func TestCallCutOffByWindowEndKeepsTheLastReason(t *testing.T) {
	for _, adminOff := range []bool{true, false} {
		x := fixture(t)
		s := baseSite()
		i := observationIntent(x, s, adminOff, "edit-route")
		x.e.activated.Store(i.Request.ID, x.now.Add(-6800*time.Millisecond))
		x.execFn = func(args []string) hostio.ExecResult {
			if strings.Contains(args[len(args)-1], "/config/") {
				return httpResult(200, configFor(s))
			}
			return httpResult(200, "")
		}
		x.fault = func(_ http.ResponseWriter, r *http.Request) bool {
			if !strings.HasSuffix(r.URL.Path, "/start") {
				return false
			}
			cmd := x.execs[strings.Split(apiVersion.ReplaceAllString(r.URL.Path, ""), "/")[2]]
			if adminOff || strings.Contains(cmd[len(cmd)-1], "/config/") {
				time.Sleep(400 * time.Millisecond) // the probe (admin off) or readback outlives the window
			}
			return false
		}
		o, err := x.e.Observe(t.Context(), i, false)
		require.NoError(t, err)
		expected := proxy.Observation{State: dto.AppliedUnverified, Reason: dto.NoObservableDifference}
		if !adminOff {
			expected.Probe = &dto.ProbeEvidence{Client: dto.WgetClient, Reason: dto.ProbeImageUnavailable}
		}
		require.Equal(t, expected, o, "admin off: %v", adminOff)
	}
}
