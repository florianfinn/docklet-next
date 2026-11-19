// Source: docklet-next/issues/493
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original stdlib-only HTTP/TLS probe and its isolated contract tests.
// Copyright 2026 Docklet contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

var version = "development"

const bodyLimit = 65536

type bodyResult struct {
	SHA256    string `json:"sha256"`
	Truncated bool   `json:"truncated"`
}
type peerResult struct {
	Subject   string   `json:"subject"`
	Issuer    string   `json:"issuer"`
	SANs      []string `json:"sans"`
	NotBefore string   `json:"notBefore"`
	NotAfter  string   `json:"notAfter"`
	SHA256    string   `json:"sha256"`
}
type verification struct {
	ChainTrusted  bool   `json:"chainTrusted"`
	HostnameMatch bool   `json:"hostnameMatch"`
	Validity      string `json:"validity"`
	Error         string `json:"error,omitempty"`
}
type tlsResult struct {
	Version string       `json:"version"`
	SNI     string       `json:"sni"`
	Peer    *peerResult  `json:"peer,omitempty"`
	Chain   []string     `json:"chain"`
	Verify  verification `json:"verify"`
}
type result struct {
	V       int               `json:"v"`
	Status  *int              `json:"status,omitempty"`
	Headers map[string]string `json:"headers,omitzero"`
	Body    *bodyResult       `json:"body,omitempty"`
	TLS     *tlsResult        `json:"tls,omitempty"`
	Error   string            `json:"error,omitempty"`
}
type options struct {
	connect, host, target, sni, roots string
	tls                               bool
	timeout                           time.Duration
	maxBody                           int
}

func main() { os.Exit(run(os.Args[1:], os.Stdout)) }

func run(args []string, out io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		return emit(out, struct {
			V       int    `json:"v"`
			Version string `json:"version"`
		}{1, version}, 0)
	}
	if len(args) == 1 && args[0] == "selftest" {
		return selftest(out)
	}
	o, err := parse(args)
	if err != nil {
		return emit(out, result{V: 1, Error: "usage"}, 2)
	}
	r, code := probe(o)
	return emit(out, r, code)
}

func emit(out io.Writer, value any, code int) int {
	// Fixed field counts and byte limits bound escaped output; this is a final fail-closed guard.
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if enc.Encode(value) != nil || b.Len() > 32768 {
		b.Reset()
		b.WriteString(`{"v":1,"error":"output-limit"}` + "\n")
		code = 3
	}
	if _, err := io.WriteString(out, b.String()); err != nil {
		return 3
	}
	return code
}

func parse(args []string) (options, error) {
	var o options
	bad := errors.New("usage")
	if len(args) == 0 || args[0] != "http" {
		return o, bad
	}
	f := flag.NewFlagSet("http", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.connect, "connect", "", "")
	f.StringVar(&o.host, "host", "", "")
	f.StringVar(&o.target, "target", "", "")
	f.StringVar(&o.sni, "sni", "", "")
	f.StringVar(&o.roots, "roots", "", "")
	f.BoolVar(&o.tls, "tls", false, "")
	f.DurationVar(&o.timeout, "timeout", 0, "")
	f.IntVar(&o.maxBody, "max-body", 0, "")
	if f.Parse(args[1:]) != nil || f.NArg() != 0 {
		return o, bad
	}
	h, p, err := net.SplitHostPort(o.connect)
	ip := net.ParseIP(h)
	port, e := strconv.Atoi(p)
	if err != nil || e != nil || strings.HasPrefix(p, "+") || ip == nil || !ip.IsLoopback() || port < 1 || port > 65535 {
		return o, bad
	}
	host, ok := hostName(o.host)
	if !ok || o.timeout <= 0 || o.timeout > 10*time.Second || o.maxBody != bodyLimit {
		return o, bad
	}
	if len(o.target) == 0 || len(o.target) > 2048 || o.target[0] != '/' || !printable(o.target) {
		return o, bad
	}
	// The request line must carry the target byte for byte, so anything Go would re-encode is refused.
	u, err := url.ParseRequestURI(o.target)
	if err != nil || u.IsAbs() || u.Host != "" || strings.Contains(o.target, "#") || u.RequestURI() != o.target {
		return o, bad
	}
	if !o.tls && o.sni != "" {
		return o, bad
	}
	if o.tls {
		if o.sni == "" {
			o.sni = host
		}
		if !dnsName(o.sni) || net.ParseIP(o.sni) != nil {
			return o, bad
		}
	}
	if o.roots != "" && (!o.tls || os.Getenv("DOCKLET_PROBE_TEST") != "1") {
		return o, bad
	}
	return o, nil
}

func printable(s string) bool {
	for _, c := range s {
		if c <= 32 || c >= 127 {
			return false
		}
	}
	return true
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
func hostName(s string) (string, bool) {
	if len(s) == 0 || len(s) > 253 || !printable(s) {
		return "", false
	}
	h := s
	if strings.HasPrefix(s, "[") || strings.Count(s, ":") == 1 {
		var p string
		var err error
		h, p, err = net.SplitHostPort(s)
		// A bracketed IPv6 literal without a port is a valid Host field.
		if err != nil {
			if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
				h = s[1 : len(s)-1]
			} else {
				return "", false
			}
		} else {
			n, e := strconv.Atoi(p)
			if e != nil || strings.HasPrefix(p, "+") || n < 1 || n > 65535 {
				return "", false
			}
		}
	}
	if strings.HasPrefix(s, "[") && net.ParseIP(h) == nil {
		return "", false
	}
	if strings.Contains(h, ":") && !strings.HasPrefix(s, "[") {
		return "", false
	}
	return h, net.ParseIP(h) != nil || dnsName(h)
}

func probe(o options) (result, int) {
	r := result{V: 1}
	if o.tls {
		r.TLS = &tlsResult{SNI: o.sni, Chain: []string{}, Verify: verification{Validity: "valid", Error: "tls-handshake"}}
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if o.roots != "" {
		pem, e := os.ReadFile(o.roots)
		roots = x509.NewCertPool()
		if e != nil || !roots.AppendCertsFromPEM(pem) {
			return result{V: 1, Error: "usage"}, 2
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	transport := &http.Transport{
		Proxy: nil, DisableKeepAlives: true, DisableCompression: true,
		MaxResponseHeaderBytes: 64 * 1024,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", o.connect)
		},
		// Verification is reported independently so untrusted HTTPS still yields an HTTP answer.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: o.sni},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	u, _ := url.ParseRequestURI(o.target)
	u.Scheme = "http"
	if o.tls {
		u.Scheme = "https"
	}
	u.Host = o.connect
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Host = o.host
	// Transport trace callbacks may finish after a cancelled request returns.
	var handshake struct {
		sync.Mutex
		failed bool
		state  *tls.ConnectionState
	}
	trace := &httptrace.ClientTrace{TLSHandshakeDone: func(state tls.ConnectionState, e error) {
		handshake.Lock()
		defer handshake.Unlock()
		handshake.failed = e != nil
		if e == nil {
			handshake.state = &state
		}
	}}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := client.Do(req)
	handshake.Lock()
	handshakeFailed, state := handshake.failed, handshake.state
	handshake.Unlock()
	if state != nil {
		r.TLS = describeTLS(*state, o.sni, roots, time.Now())
	}
	if err != nil {
		r.Error = transportError(err, handshakeFailed)
		return r, 3
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(o.maxBody)+1))
	if err != nil {
		r.Error = "body-read"
		return r, 3
	}
	r.Status = &resp.StatusCode
	r.Headers = map[string]string{}
	for _, name := range []string{"location", "server", "content-type", "content-length", "www-authenticate"} {
		values := resp.Header.Values(name)
		if len(values) > 0 {
			r.Headers[name] = bounded(strings.TrimSpace(values[len(values)-1]), 1024)
		}
	}
	truncated := len(data) > o.maxBody
	if truncated {
		data = data[:o.maxBody]
	}
	r.Body = &bodyResult{SHA256: hash(data), Truncated: truncated}
	return r, 0
}
func transportError(err error, handshakeFailed bool) string {
	var n net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &n) && n.Timeout() {
		return "timeout"
	}
	if handshakeFailed {
		return "tls-handshake"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "connect-refused"
	}
	return "protocol"
}
func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func bounded(s string, n int) string {
	// Remove control bytes and invalid UTF-8 before JSON encoding; cap without splitting a rune.
	s = strings.ToValidUTF8(s, "?")
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return '?'
		}
		return r
	}, s)
	if len(s) > n {
		s = s[:n]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}
func describeTLS(state tls.ConnectionState, sni string, roots *x509.CertPool, now time.Time) *tlsResult {
	r := &tlsResult{Version: tls.VersionName(state.Version), SNI: sni, Chain: []string{}, Verify: verification{Validity: "valid"}}
	if len(state.PeerCertificates) == 0 {
		r.Verify.Error = "tls-handshake"
		return r
	}
	leaf := state.PeerCertificates[0]
	sans := []string{}
	for _, name := range leaf.DNSNames {
		if len(sans) == 32 {
			break
		}
		sans = append(sans, bounded(name, 256))
	}
	for _, ip := range leaf.IPAddresses {
		if len(sans) == 32 {
			break
		}
		sans = append(sans, bounded(ip.String(), 256))
	}
	for _, name := range leaf.EmailAddresses {
		if len(sans) == 32 {
			break
		}
		sans = append(sans, bounded(name, 256))
	}
	for _, uri := range leaf.URIs {
		if len(sans) == 32 {
			break
		}
		sans = append(sans, bounded(uri.String(), 256))
	}
	r.Peer = &peerResult{Subject: bounded(leaf.Subject.String(), 256), Issuer: bounded(leaf.Issuer.String(), 256), SANs: sans,
		NotBefore: leaf.NotBefore.UTC().Format(time.RFC3339), NotAfter: leaf.NotAfter.UTC().Format(time.RFC3339), SHA256: hash(leaf.Raw)}
	intermediates := x509.NewCertPool()
	for i, cert := range state.PeerCertificates {
		if i < 4 {
			r.Chain = append(r.Chain, hash(cert.Raw))
		}
		if i > 0 {
			intermediates.AddCert(cert)
		}
	}
	_, err := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, CurrentTime: now})
	r.Verify.ChainTrusted = err == nil
	r.Verify.HostnameMatch = leaf.VerifyHostname(sni) == nil
	if now.Before(leaf.NotBefore) {
		r.Verify.Validity = "not-yet-valid"
	} else if now.After(leaf.NotAfter) {
		r.Verify.Validity = "expired"
	}
	// Fixed codes avoid unbounded or environment-specific x509 error strings.
	if err != nil {
		r.Verify.Error = "chain-untrusted"
	} else if !r.Verify.HostnameMatch {
		r.Verify.Error = "hostname-mismatch"
	}
	return r
}
