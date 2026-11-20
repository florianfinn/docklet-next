// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

package hostio

import (
	"encoding/json/v2"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	dto "github.com/florianfinn/docklet-next/types/proxy"
)

const MaxProbeJSON = 32 << 10
const MaxProbeStderr = 4 << 10
const MaxProbeBody = 64 << 10

var probeHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

type probeJSON struct {
	V       int               `json:"v"`
	Status  *int              `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    *struct {
		SHA256    string `json:"sha256"`
		Truncated *bool  `json:"truncated"`
	} `json:"body"`
	TLS *struct {
		Version string               `json:"version"`
		SNI     string               `json:"sni"`
		Peer    *dto.CertificatePeer `json:"peer"`
		Chain   []string             `json:"chain"`
		Verify  struct {
			ChainTrusted  *bool                   `json:"chainTrusted"`
			HostnameMatch *bool                   `json:"hostnameMatch"`
			Validity      dto.CertificateValidity `json:"validity"`
			Error         string                  `json:"error"`
		} `json:"verify"`
	} `json:"tls"`
	Error string `json:"error"`
}

func boundedText(s string, limit int) bool {
	return len(s) <= limit && !strings.ContainsFunc(s, unicode.IsControl)
}
func dnsName(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(s, "."), ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

// ProbeArgs preserves Host, SNI and the exact request target independently. Only
// loopback connections in the target container's network namespace are allowed.
func ProbeArgs(host, rawURL, sni string) ([]string, bool, error) {
	fail := func() ([]string, bool, error) { return nil, false, dto.Fail(dto.InvalidInput, 422) }
	if !SafeValue(host) || len(host) > 253 || strings.ContainsAny(host, " /\\") {
		return fail()
	}
	name := host
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		name = host[1 : len(host)-1]
	} else if strings.Contains(host, ":") {
		var err error
		var port string
		name, port, err = net.SplitHostPort(host)
		number, portErr := strconv.Atoi(port)
		if portErr != nil || strings.HasPrefix(port, "+") || number < 1 || number > 65535 {
			return fail()
		}
		if err != nil {
			return fail()
		}
	}
	if net.ParseIP(name) == nil && !dnsName(name) {
		return fail()
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fail()
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return fail()
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	target := u.RequestURI()
	if len(target) > 2048 || !strings.HasPrefix(target, "/") || strings.ContainsAny(target, " \r\n\x00") {
		return fail()
	}
	args := []string{"/usr/bin/docklet-probe", "http", "--connect", net.JoinHostPort(u.Hostname(), port), "--host", host, "--target", target, "--timeout", "10s", "--max-body", "65536"}
	tls := u.Scheme == "https"
	if tls {
		if sni == "" {
			sni = name
		}
		if !dnsName(sni) || net.ParseIP(sni) != nil {
			return fail()
		}
		args = append(args, "--tls", "--sni", sni)
	} else if sni != "" {
		return fail()
	}
	return args, tls, nil
}

// ParseHelperProbe rejects contradictory evidence rather than changing clients.
// TLS verification flags are copied independently and never affect HTTP status.
func ParseHelperProbe(res ExecResult, tls bool) (Response, error) {
	bad := func() (Response, error) { return Response{}, dto.Fail(dto.NotObservable, 409) }
	var v probeJSON
	if len(res.Stdout) > MaxProbeJSON || len(res.Stderr) > MaxProbeStderr || json.Unmarshal(res.Stdout, &v) != nil || v.V != 1 || (v.TLS != nil) != tls {
		return bad()
	}
	if res.ExitCode == 0 {
		if v.Status == nil || *v.Status < 100 || *v.Status > 599 || v.Error != "" || v.Body == nil || v.Body.Truncated == nil || !probeHash.MatchString(v.Body.SHA256) || v.Headers == nil {
			return bad()
		}
	} else if res.ExitCode == 3 {
		if v.Error == "" || !boundedText(v.Error, 256) || v.Status != nil || v.Body != nil || len(v.Headers) != 0 {
			return bad()
		}
	} else {
		return bad()
	}
	out := Response{Headers: map[string]string{}}
	if v.Status != nil {
		out.Status = *v.Status
		out.BodyHash = v.Body.SHA256
		out.Truncated = *v.Body.Truncated
	}
	for name, value := range v.Headers {
		name = strings.ToLower(strings.TrimSpace(name))
		if !probeHeaders[name] || !boundedText(value, 1024) {
			return bad()
		}
		out.Headers[name] = strings.TrimSpace(value)
	}
	if t := v.TLS; t != nil {
		if !dnsName(t.SNI) || net.ParseIP(t.SNI) != nil || len(t.Chain) > 4 || t.Verify.ChainTrusted == nil || t.Verify.HostnameMatch == nil || !boundedText(t.Verify.Error, 256) {
			return bad()
		}
		for _, hash := range t.Chain {
			if !probeHash.MatchString(hash) {
				return bad()
			}
		}
		if t.Peer == nil {
			if out.Status != 0 || t.Version != "" || len(t.Chain) != 0 || *t.Verify.ChainTrusted || *t.Verify.HostnameMatch || t.Verify.Error != "tls-handshake" {
				return bad()
			}
		} else {
			p := t.Peer
			if t.Version == "" || !boundedText(t.Version, 32) || len(t.Chain) == 0 || t.Chain[0] != p.SHA256 || !boundedText(p.Subject, 256) || !boundedText(p.Issuer, 256) || len(p.SANs) > 32 || !probeHash.MatchString(p.SHA256) || p.NotBefore.IsZero() || p.NotAfter.IsZero() || p.NotAfter.Before(p.NotBefore) {
				return bad()
			}
			for _, san := range p.SANs {
				if !boundedText(san, 256) {
					return bad()
				}
			}
			if t.Verify.Validity != dto.CertificateValid && t.Verify.Validity != dto.CertificateExpired && t.Verify.Validity != dto.CertificateNotYetValid {
				return bad()
			}
			out.Certificate = &dto.CertificateObservation{ObservedAt: time.Now().UTC(), Route: dto.CertificateRoute{SNI: t.SNI}, Peer: *p, ChainTrusted: *t.Verify.ChainTrusted, HostnameMatch: *t.Verify.HostnameMatch, Validity: t.Verify.Validity, Error: t.Verify.Error}
		}
	}
	return out, nil
}
