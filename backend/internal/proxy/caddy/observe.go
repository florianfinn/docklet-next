// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original first-response and Caddy admin verification.

package caddy

import (
	"context"
	"encoding/json/v2"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
)

// adminURL returns the loopback URL and the Host header Caddy's admin endpoint
// accepts: its listen authority (host and port), e.g. localhost:2019.
func adminURL(address string) (string, string, error) {
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	u, err := url.Parse(address)
	if err != nil {
		return "", "", dto.Fail(dto.AdminUnavailable, 409)
	}
	if u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return "", "", dto.Fail(dto.AdminUnavailable, 409)
	}
	host := u.Hostname()
	if host != "localhost" && host != "127.0.0.1" && host != "0.0.0.0" && host != "" && host != "::1" && host != "::" {
		return "", "", dto.Fail(dto.AdminUnavailable, 409)
	}
	port := u.Port()
	if port == "" {
		port = "2019"
	}
	authority := host
	if authority == "" || authority == "0.0.0.0" || authority == "::" {
		authority = "localhost"
	}
	u.Host = net.JoinHostPort("127.0.0.1", port)
	u.Path = "/config/"
	return u.String(), net.JoinHostPort(authority, port), nil
}
func (e *engine) admin(ctx context.Context, i dto.Instance, address string) (map[string]any, error) {
	u, authority, err := adminURL(address)
	if err != nil {
		return nil, err
	}
	res, err := e.runtime.Exec(ctx, i, "admin", "", authority, u)
	if err != nil {
		// An uncertain exec or a failed Docker call (no command ran) is no admin answer.
		if reason(err, "") == dto.ExecRunning || proxy.Transient(err) {
			return nil, err
		}
		return nil, dto.Fail(dto.AdminUnavailable, 409)
	}
	response, err := hostio.ParseProbe(res, nil)
	if err != nil || response.Status != 200 || response.Followed {
		return nil, dto.Fail(dto.AdminUnavailable, 409)
	}
	var config map[string]any
	if err := json.Unmarshal(res.Stdout, &config); err != nil {
		return nil, dto.Fail(dto.AdminUnavailable, 409)
	}
	if config == nil {
		return nil, dto.Fail(dto.AdminUnavailable, 409)
	}
	return config, nil
}
func (e *engine) diverged(ctx context.Context, i dto.Instance, inv dto.Inventory) (bool, error) {
	running, err := e.admin(ctx, i, inv.AdminAddress)
	if err != nil {
		return false, err
	}
	res, err := e.runtime.Exec(ctx, i, "adapt", inv.RootFile, "", "")
	if err = commandError(res, err, inv.RootFile); err != nil {
		return false, err
	}
	var adapted map[string]any
	if err := json.Unmarshal(res.Stdout, &adapted); err != nil {
		return false, problem(dto.InvalidGraph, inv.RootFile, 0, "invalid adapted JSON")
	}
	if adapted == nil {
		return false, problem(dto.InvalidGraph, inv.RootFile, 0, "empty adapted JSON")
	}
	a, err := json.Marshal(running, json.Deterministic(true))
	if err != nil {
		return false, err
	}
	b, err := json.Marshal(adapted, json.Deterministic(true))
	if err != nil {
		return false, err
	}
	return string(a) != string(b), nil
}
func (e *engine) probe(ctx context.Context, i dto.Instance, s Site) (hostio.Response, error) {
	if strings.HasPrefix(s.Domain, "*.") {
		return hostio.Response{}, dto.Fail(dto.NotObservable, 409)
	}
	scheme := "https"
	if s.TLS == "off" {
		scheme = "http"
	}
	u := scheme + "://127.0.0.1" + s.Path
	if s.Path == "" {
		u += "/"
	}
	if runner, ok := e.runtime.(interface {
		Probe(context.Context, dto.Instance, string, string) (hostio.Response, error)
	}); ok {
		response, err := runner.Probe(ctx, i, s.Domain, u)
		if response.Certificate != nil {
			response.Certificate.Route.Name = s.Name
		}
		return response, err
	}
	res, err := e.runtime.Exec(ctx, i, "probe", "", s.Domain, u)
	return hostio.ParseProbe(res, err)
}
func routing(before, after hostio.Response, kind string) bool {
	if hostio.GatewayError(after.Status) || !hostio.Differs(before, after) {
		return false
	}
	// Caddy has no distinctive no-route answer: an unmatched host gets an empty 200
	// (baseline B0) or a catch-all site's answer. A create, edit or delete therefore
	// counts when the first response differs and is not a gateway error (AUTO-4). A
	// delete needs an answer before the change, a create or edit one after it.
	if kind == "delete-route" {
		return before.Status != 0
	}
	return after.Status != 0
}
func unverified(r dto.Reason) proxy.Observation {
	return proxy.Observation{State: dto.AppliedUnverified, Reason: r}
}
func (e *engine) Observe(ctx context.Context, i proxy.Intent, _ bool) (observation proxy.Observation, observeErr error) {
	ctx = hostio.WithProbeOperation(ctx, i.Request.ID)
	s, err := decode(i)
	if err != nil {
		return proxy.Observation{}, err
	}
	if err = proxy.EnsureProbePrepared(ctx, e.runtime, i.Request.Instance); err != nil {
		return proxy.Observation{}, err
	}
	start := e.now()
	if at, ok := e.activated.Load(i.Request.ID); ok {
		start = at.(time.Time)
	}
	deadline := start.Add(7 * time.Second)
	remaining := deadline.Sub(e.now())
	if remaining <= 0 {
		return unverified(dto.NoObservableDifference), nil
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, remaining)
	defer cancel()
	// Runtime.Epoch and Logs fence real Docker identity. For an acknowledged
	// restart, capture the new epoch rather than trusting the registration epoch.
	// A transient read is no evidence: without a restart the registration epoch is
	// expected; after one, the first successful round read fixes it.
	running, epoch, err := e.runtime.Epoch(ctx, i.Request.Instance)
	if proxy.Transient(err) {
		running, epoch, err = true, "", nil
		if !i.Restart {
			epoch = i.Request.Instance.Identity.StartedAt
		}
	}
	if err != nil || !running {
		return unverified(dto.ContainerChanged), nil
	}
	if !i.Restart && epoch != i.Request.Instance.Identity.StartedAt {
		return unverified(dto.ContainerChanged), nil
	}
	site := s.NewSite
	if site == nil {
		site = s.OldSite
	}
	var certificate *dto.CertificateObservation
	var probeEvidence *dto.ProbeEvidence
	defer func() {
		if observeErr == nil && observation.Reason != dto.ContainerChanged {
			observation.Certificate, observation.Probe = certificate, probeEvidence
		}
	}()
	lastReason := dto.NoObservableDifference
	probeFailed := false // a later successful probe clears a reason a failed probe set
	for {
		if !e.now().Before(deadline) {
			return unverified(lastReason), nil
		}
		// A backend request error is not a Caddy configuration rejection.
		// Validate/reload decide rejection; logs still fence availability/epoch.
		err = e.runtime.Logs(ctx, i.Request.Instance, i.PublishedAt, func(string) error { return nil })
		if err != nil {
			if ctx.Err() != nil && parent.Err() == nil {
				return unverified(lastReason), nil
			}
			return unverified(reason(err, dto.LogsUnavailable)), nil
		}
		routed := false
		unobservable := site == nil
		probeReason := s.ProbeReason
		if site != nil {
			after, probeErr := e.probe(ctx, i.Request.Instance, *site)
			if probeErr != nil && hostio.ProbeTransient(probeErr) && ctx.Err() == nil {
				return proxy.Observation{}, probeErr
			}
			if probeErr == nil {
				probeEvidence = after.Probe
				if after.Certificate != nil {
					certificate = after.Certificate
				}
			}
			if probeErr != nil && ctx.Err() != nil && parent.Err() == nil {
				return unverified(lastReason), nil // the window ended during the probe
			}
			unobservable = probeErr == nil && after.Status == 0 || reason(probeErr, "") == dto.NotObservable
			if probeErr != nil {
				probeReason, probeFailed = reason(probeErr, dto.NotObservable), true
			} else {
				// Routing evidence needs a successful baseline probe, not a failed exec.
				routed = s.ProbeReason == "" && routing(s.Before, after, i.Request.Change.Kind)
				if probeFailed {
					lastReason, probeFailed = dto.NoObservableDifference, false
				}
			}
		}
		configOK := s.AdminOff
		if !s.AdminOff {
			config, adminErr := e.admin(ctx, i.Request.Instance, s.Admin)
			if adminErr != nil && ctx.Err() != nil && parent.Err() == nil {
				return unverified(lastReason), nil // the window ended during the readback
			}
			if adminErr != nil {
				lastReason = reason(adminErr, dto.AdminUnavailable)
			} else {
				if s.NewSite != nil {
					configOK = containsSite(config, *s.NewSite, true)
				} else if s.OldSite != nil {
					configOK = !containsSite(config, *s.OldSite, false)
				} else { // An import insertion proves the complete adapted graph, including an empty set.
					inv := dto.Inventory{RootFile: s.Root, AdminAddress: s.Admin}
					differs, err := e.diverged(ctx, i.Request.Instance, inv)
					configOK = err == nil && !differs
				}
				if configOK && !routed {
					lastReason = probeReason
				}
			}
		}
		if probeReason != "" {
			lastReason = probeReason
		}
		if lastReason == "" {
			lastReason = dto.NoObservableDifference
		}
		currentRunning, currentEpoch, epochErr := e.runtime.Epoch(ctx, i.Request.Instance)
		if epochErr != nil && ctx.Err() != nil && parent.Err() == nil {
			return unverified(lastReason), nil // the window ended, identity is unknown
		}
		if proxy.Transient(epochErr) {
			// Identity unconfirmed this round: no verification from it.
			if err = e.wait(ctx, 100*time.Millisecond); err != nil {
				if parent.Err() == nil {
					return unverified(lastReason), nil
				}
				return proxy.Observation{}, err
			}
			continue
		}
		if epoch == "" && epochErr == nil && currentRunning {
			epoch = currentEpoch
		}
		if epochErr != nil || !currentRunning || currentEpoch != epoch {
			return unverified(dto.ContainerChanged), nil
		}
		if !e.now().Before(deadline) {
			return unverified(lastReason), nil
		}
		if configOK && routed {
			return proxy.Observation{State: dto.Verified, Evidence: dto.Routing}, nil
		}
		if !s.AdminOff && configOK && (unobservable || s.Rename) {
			return proxy.Observation{State: dto.Verified, Evidence: dto.Configuration}, nil
		}
		if err = e.wait(ctx, 100*time.Millisecond); err != nil {
			// The observation window ended; only a cancelled operation context is uncertain.
			if parent.Err() == nil {
				return unverified(lastReason), nil
			}
			return proxy.Observation{}, err
		}
	}
}

func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func array(v any) []any           { a, _ := v.([]any); return a }
func stringsIn(v any, want string) bool {
	for _, s := range array(v) {
		if s == want {
			return true
		}
	}
	return false
}

// containsSite follows each route branch: a host, path and reverse proxy must
// belong to the same effective branch, not unrelated JSON subtrees.
func containsSite(config map[string]any, s Site, targets bool) bool {
	var walk func([]any, bool, bool) bool
	walk = func(routes []any, host, pathOK bool) bool {
		for _, raw := range routes {
			r := object(raw)
			matches := array(r["match"])
			if len(matches) == 0 {
				matches = []any{map[string]any{}}
			}
			for _, m := range matches {
				match := object(m)
				h, p := host, pathOK
				if hosts, ok := match["host"]; ok {
					if !stringsIn(hosts, s.Domain) {
						continue
					}
					h = true
				}
				if paths, ok := match["path"]; ok {
					if s.Path == "" || !stringsIn(paths, s.Path+"*") {
						continue
					}
					p = true
				}
				restricted := false
				for key := range match {
					if key != "host" && key != "path" {
						restricted = true
					}
				}
				if restricted {
					continue
				}
				for _, rawHandler := range array(r["handle"]) {
					handler := object(rawHandler)
					if handler["handler"] == "reverse_proxy" && h && p {
						if !targets || upstreamsEqual(handler, s.Upstreams) {
							return true
						}
					}
					if walk(array(handler["routes"]), h, p) {
						return true
					}
				}
			}
		}
		return false
	}
	servers := object(object(object(config["apps"])["http"])["servers"])
	for _, server := range servers {
		if walk(array(object(server)["routes"]), false, s.Path == "") {
			return true
		}
	}
	return false
}
func upstreamsEqual(handler map[string]any, want []string) bool {
	got := []string{}
	for _, v := range array(handler["upstreams"]) {
		dial, _ := object(v)["dial"].(string)
		got = append(got, dial)
	}
	expected := []string{}
	tls := false
	for _, v := range want {
		u, err := url.Parse(v)
		if err != nil {
			return false
		}
		host := u.Host
		if u.Port() == "" {
			port := "80"
			if u.Scheme == "https" {
				port = "443"
			}
			host = net.JoinHostPort(u.Hostname(), port)
		}
		expected = append(expected, host)
		if u.Scheme == "https" {
			tls = true
		}
	}
	if len(got) != len(expected) {
		return false
	}
	slices.Sort(got)
	slices.Sort(expected)
	transport := object(handler["transport"])
	if protocol, ok := transport["protocol"]; ok && protocol != "http" {
		return false
	}
	hasTLS := object(transport["tls"]) != nil
	return slices.Equal(got, expected) && tls == hasTLS
}

func (e *engine) CheckCertificate(ctx context.Context, i dto.Instance, q dto.CertificateCheckRequest) (dto.CertificateObservation, error) {
	read, err := e.Read(ctx, i)
	if err != nil {
		return dto.CertificateObservation{}, err
	}
	route, err := proxy.CertificateRoute(read, q)
	if err != nil {
		return dto.CertificateObservation{}, err
	}
	content := ""
	for _, f := range read.Files {
		if f.Path == q.File {
			content = f.Content
		}
	}
	site, err := ParseSite(content)
	if err != nil || site.TLS == "off" || site.Domain != q.Host {
		return dto.CertificateObservation{}, dto.Fail(dto.CertificateUnavailable, 409)
	}
	if err = proxy.PrewarmProbe(ctx, e.runtime, i); err != nil {
		return dto.CertificateObservation{}, err
	}
	response, err := e.probe(ctx, i, site)
	if err != nil {
		return dto.CertificateObservation{}, err
	}
	after, err := e.Read(ctx, i)
	if err != nil {
		return dto.CertificateObservation{}, err
	}
	if _, err = proxy.CertificateRoute(after, q); err != nil {
		return dto.CertificateObservation{}, err
	}
	return proxy.CertificateResult(response, route.Name, "")
}
