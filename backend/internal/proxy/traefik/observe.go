// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Traefik v3 engine.

package traefik

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
)

func routeProbes(r dto.Route, inv dto.Inventory) []probe {
	calls, err := parseRule(r.Rule)
	if err != nil {
		return []probe{{Route: r.ID, Reason: dto.NotObservable}}
	}
	hosts := []string{}
	requestPath := "/"
	for _, c := range calls {
		switch c.name {
		case "Host":
			hosts = append(hosts, c.args[0])
		case "HostRegexp":
			expression := c.args[0]
			prefix := "^[a-z0-9-]+\\."
			if strings.HasPrefix(expression, prefix) && strings.HasSuffix(expression, "$") {
				hosts = append(hosts, "docklet-probe."+strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(expression, prefix), "$"), "\\.", "."))
			}
		case "Path", "PathPrefix":
			requestPath = c.args[0]
		}
	}
	out := []probe{}
	for _, ep := range inv.EntryPoints {
		if len(r.EntryPoints) > 0 && !contains(r.EntryPoints, ep.Name) {
			continue
		}
		if strings.HasSuffix(ep.Address, "/udp") {
			continue
		}
		_, port, err := net.SplitHostPort(strings.TrimSuffix(ep.Address, "/tcp"))
		if err != nil {
			continue
		}
		scheme := "http"
		if ep.TLS {
			scheme = "https"
		}
		for _, host := range hosts {
			u := url.URL{Scheme: scheme, Host: net.JoinHostPort("127.0.0.1", port), Path: requestPath}
			out = append(out, probe{Route: r.ID, Host: host, URL: u.String()})
		}
	}
	if len(out) == 0 {
		return []probe{{Route: r.ID, Reason: dto.NotObservable}}
	}
	return out
}
func markerProbe(id string, inv dto.Inventory) probe {
	for _, ep := range inv.EntryPoints {
		if ep.TLS || strings.HasSuffix(ep.Address, "/udp") {
			continue
		}
		_, port, err := net.SplitHostPort(strings.TrimSuffix(ep.Address, "/tcp"))
		if err == nil {
			return probe{Route: ep.Name, Host: id + ".docklet-check.invalid", URL: "http://127.0.0.1:" + port + "/"}
		}
	}
	return probe{Reason: dto.NotObservable}
}
func (e *engine) probe(ctx context.Context, i dto.Instance, p probe) (hostio.Response, error) {
	res, err := e.run.Exec(ctx, i, "probe", "", p.Host, p.URL)
	return hostio.ParseProbe(res, err)
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func providerError(line string) bool {
	lower := strings.ToLower(ansi.ReplaceAllString(line, ""))
	// Provider parser errors and router-build failures appear at INFO and ERROR;
	// JSON error records may have no message field at all.
	failure := strings.Contains(lower, `"error":`) || strings.Contains(lower, " error=") || strings.Contains(lower, " err ") || strings.Contains(lower, "level=error") || strings.Contains(lower, `"level":"error"`) || strings.HasPrefix(lower, "err ")
	config := strings.Contains(lower, "providername=file") || strings.Contains(lower, `"providername":"file"`) || strings.Contains(lower, "@file") || strings.Contains(lower, "file provider") || strings.Contains(lower, "configuration file")
	return failure && config
}
func (e *engine) checkLogs(ctx context.Context, i dto.Instance, since time.Time) error {
	if since.IsZero() {
		return dto.Fail(dto.LogsUnavailable, 409)
	}
	return e.run.Logs(ctx, i, since, func(batch string) error {
		for _, line := range strings.Split(batch, "\n") {
			line = strings.TrimPrefix(line, "[STDERR] ")
			text := line
			if head, rest, ok := strings.Cut(line, " "); ok {
				if stamp, err := time.Parse(time.RFC3339Nano, head); err == nil {
					if stamp.Before(since) {
						continue
					}
					text = rest
				}
			}
			if providerError(text) {
				return dto.Fail(dto.ProxyRejected, 422)
			}
		}
		return nil
	})
}
func noRoute(r hostio.Response) bool {
	h := sha256.Sum256([]byte("404 page not found\n"))
	return r.Status == 404 && (r.BodyHash == hex.EncodeToString(h[:]) || (r.Headers["content-length"] == "19" && strings.HasPrefix(r.Headers["content-type"], "text/plain")))
}
func routingEvidence(kind string, before, after hostio.Response) bool {
	if after.Status == 0 || hostio.GatewayError(after.Status) {
		return false
	}
	switch kind {
	case "create-route":
		return noRoute(before) && !noRoute(after)
	case "delete-route":
		return before.Status != 0 && !noRoute(before) && noRoute(after)
	default:
		return before.Status != 0 && !noRoute(after) && hostio.Differs(before, after)
	}
}
func unverified(code dto.Reason) proxy.Observation {
	return proxy.Observation{State: dto.AppliedUnverified, Reason: code}
}
func (e *engine) Observe(ctx context.Context, in proxy.Intent, marked bool) (proxy.Observation, error) {
	s, err := state(in)
	if err != nil {
		return proxy.Observation{}, err
	}
	budget := time.Duration(s.Inventory.ThrottleMillis)*2*time.Millisecond + 10*time.Second
	deadline := e.now().Add(budget)
	// A recovery call starts a fresh observation window; publication time still
	// bounds log evidence from the original operation.
	lastReason := dto.NoObservableDifference
	for {
		if err = e.checkLogs(ctx, in.Request.Instance, in.PublishedAt); err != nil {
			code := reason(err, dto.LogsUnavailable)
			if code == dto.ProxyRejected {
				return proxy.Observation{Restore: true, Reason: code}, nil
			}
			return unverified(code), nil
		}
		all := len(s.Probes) > 0 && !s.ForceMarker
		unanswered := false
		for _, p := range s.Probes {
			if p.Reason != "" {
				all = false
				lastReason = p.Reason
				if s.Unresolved {
					unanswered = true
				}
				continue
			}
			after, probeErr := e.probe(ctx, in.Request.Instance, p)
			if probeErr != nil {
				all = false
				lastReason = reason(probeErr, dto.NotObservable)
				if s.Unresolved {
					unanswered = true
				}
				continue
			}
			if after.Status == 0 || noRoute(after) {
				unanswered = true
			}
			if !routingEvidence(in.Request.Change.Kind, p.Before, after) {
				all = false
			}
		}
		if all && !marked {
			return proxy.Observation{State: dto.Verified, Evidence: dto.Routing}, nil
		}
		if marked && (!s.Unresolved || !unanswered) {
			if s.Marker.Reason != "" {
				return unverified(s.Marker.Reason), nil
			}
			answer, probeErr := e.probe(ctx, in.Request.Instance, s.Marker)
			if probeErr != nil {
				lastReason = reason(probeErr, dto.NotObservable)
			} else if answer.Status != 0 && answer.Status != 404 && hostio.Differs(s.Marker.Before, answer) {
				// The intentionally closed loopback upstream gives 502: this is load
				// evidence only, never routing evidence for the edited route.
				if err = e.checkLogs(ctx, in.Request.Instance, in.PublishedAt); err != nil {
					if reason(err, "") == dto.ProxyRejected {
						return proxy.Observation{Restore: true, Reason: dto.ProxyRejected}, nil
					}
					return unverified(reason(err, dto.LogsUnavailable)), nil
				}
				return proxy.Observation{State: dto.Verified, Evidence: dto.Load}, nil
			}
		}
		if !e.now().Before(deadline) {
			if s.Unresolved && unanswered {
				return proxy.Observation{Restore: true, Reason: dto.ProxyRejected}, nil
			}
			if !marked && s.Marker.Reason == "" {
				return proxy.Observation{NeedsMarker: true}, nil
			}
			if marked && lastReason == dto.NoObservableDifference {
				lastReason = dto.NotObservable
			}
			if s.Marker.Reason != "" && lastReason == dto.NoObservableDifference {
				lastReason = s.Marker.Reason
			}
			return unverified(lastReason), nil
		}
		if err = e.wait(ctx, 250*time.Millisecond); err != nil {
			return proxy.Observation{}, err
		}
	}
}
func (e *engine) Mark(ctx context.Context, in proxy.Intent) error {
	s, err := state(in)
	if err != nil {
		return err
	}
	if s.Marker.Reason != "" {
		return dto.Fail(s.Marker.Reason, 409)
	}
	if in.MarkerPath == "" || !namePattern.MatchString(in.Request.ID) {
		return dto.Fail(dto.InvalidInput, 422)
	}
	// Framework owns this exact transient name and its cleanup/recovery lifecycle.
	expected := "docklet-check-" + in.Request.ID + ".yml"
	if !strings.HasSuffix(in.MarkerPath, "/"+expected) && in.MarkerPath != expected {
		return dto.Fail(dto.InvalidInput, 422)
	}
	text, err := render(map[string]any{"http": map[string]any{"routers": map[string]any{"docklet-check-" + in.Request.ID: map[string]any{"rule": "Host(`" + s.Marker.Host + "`)", "entryPoints": []string{s.Marker.Route}, "service": "docklet-check-" + in.Request.ID}}, "services": map[string]any{"docklet-check-" + in.Request.ID: backend([]string{"http://127.0.0.1:1"})}}}, 0)
	if err != nil {
		return err
	}
	text += "\n"
	w, err := e.run.Writer(ctx, in.Request.Instance)
	if err != nil {
		return err
	}
	// Reuse the protocol-owned temporary name after target publication. A file
	// provider must never observe a partially written marker.
	if err = w.Write(ctx, in.TempPath, []byte(text), 0644); err != nil {
		return err
	}
	return w.Replace(ctx, in.TempPath, in.MarkerPath)
}
