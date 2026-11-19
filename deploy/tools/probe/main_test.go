// Source: docklet-next/issues/493
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original stdlib-only HTTP/TLS probe and its isolated contract tests.
// Copyright 2026 Docklet contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func invoke(t *testing.T, args ...string) (result, int, []byte) {
	t.Helper()
	var out bytes.Buffer
	code := run(args, &out)
	b := out.Bytes()
	if len(b) > 32768 || bytes.Count(b, []byte("\n")) != 1 {
		t.Fatalf("JSON boundary: bytes=%d lines=%d", len(b), bytes.Count(b, []byte("\n")))
	}
	var r result
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if r.V != 1 {
		t.Fatalf("version: %+v", r)
	}
	if code == 0 && (r.Error != "" || r.Status == nil && (len(args) == 0 || args[0] == "http")) || code != 0 && (r.Error == "" || r.Status != nil) {
		t.Fatalf("exit/field consistency: code=%d result=%+v", code, r)
	}
	return r, code, b
}
func cli(connect string) []string {
	return []string{"http", "--connect", connect, "--host", "example.test", "--target", "/path?query=yes", "--timeout", "2s", "--max-body", "65536"}
}
func endpoint(s *httptest.Server) string {
	return strings.TrimPrefix(strings.TrimPrefix(s.URL, "https://"), "http://")
}

func TestP1RedirectNeverFollowed(t *testing.T) {
	var hits atomic.Int32
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sink" {
			hits.Add(1)
			return
		}
		http.Redirect(w, r, "/sink", http.StatusFound)
	}))
	defer redirect.Close()
	r, code, _ := invoke(t, cli(endpoint(redirect))...)
	if hits.Load() != 0 {
		t.Fatalf("redirect sink requests = %d; want 0", hits.Load())
	}
	if code != 0 || *r.Status != 302 || r.Headers["location"] != "/sink" {
		t.Fatalf("redirect response lost: %+v code=%d", r, code)
	}
}

func TestP2SNIIndependentOfHost(t *testing.T) {
	var seenSNI, seenHost string
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seenHost = r.Host }))
	s.TLS = &tls.Config{GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) { seenSNI = hello.ServerName; return nil, nil }}
	s.StartTLS()
	defer s.Close()
	args := append(cli(endpoint(s)), "--tls", "--sni", "tls.example.test")
	r, code, _ := invoke(t, args...)
	if code != 0 {
		t.Fatalf("TLS response: %+v", r)
	}
	if seenSNI != "tls.example.test" || seenHost != "example.test" {
		t.Fatalf("independent SNI/Host: SNI=%q Host=%q", seenSNI, seenHost)
	}
	if r.TLS.SNI != seenSNI {
		t.Fatal("reported SNI differs from handshake")
	}
}

func TestDefaultSNIIsHostWithoutPort(t *testing.T) {
	var seenSNI string
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	s.TLS = &tls.Config{GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) { seenSNI = hello.ServerName; return nil, nil }}
	s.StartTLS()
	defer s.Close()
	args := append(cli(endpoint(s)), "--host", "default.example.test:8443", "--tls")
	if r, code, _ := invoke(t, args...); code != 0 || seenSNI != "default.example.test" || r.TLS.SNI != seenSNI {
		t.Fatalf("default SNI=%q code=%d", seenSNI, code)
	}
}

func TestTargetSentVerbatim(t *testing.T) {
	var seen string
	s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = r.RequestURI }))
	defer s.Close()
	target := "/a%2Fb/c%20d?x=%C3%A9&y=1"
	args := append(cli(endpoint(s)), "--target", target)
	if _, code, _ := invoke(t, args...); code != 0 || seen != target {
		t.Fatalf("request target %q; want %q", seen, target)
	}
}

func TestHTTPStatusesHeadersAndProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://192.0.2.1:1")
	t.Setenv("HTTPS_PROXY", "http://192.0.2.1:1")
	t.Setenv("ALL_PROXY", "http://192.0.2.1:1")
	t.Setenv("NO_PROXY", "")
	for _, status := range []int{200, 401, 502, 503, 504} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var hits atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				if r.Method != "GET" || r.RequestURI != "/path?query=yes" || r.Host != "example.test" {
					t.Errorf("request: %s %s %s", r.Method, r.RequestURI, r.Host)
				}
				if r.Header.Get("Accept-Encoding") != "" {
					t.Error("compression changes body hash")
				}
				w.Header().Add("sErVeR", " first ")
				w.Header().Add("Server", " last ")
				w.Header().Set("WWW-Authenticate", `Basic realm="example.test"`)
				w.Header().Set("X-Private", "omit")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "body")
			}))
			defer s.Close()
			r, code, _ := invoke(t, cli(endpoint(s))...)
			if code != 0 || *r.Status != status || hits.Load() != 1 || r.Body.SHA256 != hash([]byte("body")) || r.Body.Truncated {
				t.Fatalf("HTTP answer: %+v code=%d hits=%d", r, code, hits.Load())
			}
			if r.Headers["server"] != "last" || r.Headers["www-authenticate"] != `Basic realm="example.test"` || r.Headers["x-private"] != "" {
				t.Fatalf("headers: %+v", r.Headers)
			}
		})
	}
}

func TestP10BodyAndHeaderBoundary(t *testing.T) {
	for _, size := range []int{0, bodyLimit - 1, bodyLimit, bodyLimit + 1, bodyLimit * 3} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			data := strings.Repeat("x", size)
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for _, h := range []string{"Location", "Server", "Content-Type", "WWW-Authenticate"} {
					w.Header().Set(h, strings.Repeat(`"`, 4096))
				}
				_, _ = io.WriteString(w, data)
			}))
			defer s.Close()
			r, code, _ := invoke(t, cli(endpoint(s))...)
			if code != 0 {
				t.Fatalf("code=%d result=%+v", code, r)
			}
			for name, value := range r.Headers {
				if len(value) > 1024 {
					t.Fatalf("header %s bytes=%d; want <=1024", name, len(value))
				}
			}
			if r.Body.Truncated != (size > bodyLimit) || r.Body.SHA256 != hash([]byte(data[:min(size, bodyLimit)])) {
				t.Fatalf("body boundary: %+v size=%d", r.Body, size)
			}
		})
	}
}

func TestArguments(t *testing.T) {
	good := cli("127.0.0.1:1234")
	for _, connect := range []string{"[::1]:1234", "127.2.3.4:1234"} {
		a := cli(connect)
		if _, err := parse(a); err != nil {
			t.Fatalf("valid loopback %s", connect)
		}
	}
	for _, host := range []string{"example.test:443", "192.0.2.1", "[::1]", "[::1]:443"} {
		a := append(append([]string{}, good...), "--host", host)
		if _, err := parse(a); err != nil {
			t.Fatalf("valid host %s", host)
		}
	}
	invalid := [][]string{nil, {"unknown"}, {"http"}, append(append([]string{}, good...), "extra")}
	for _, v := range []struct {
		flag   string
		values []string
	}{
		{"--connect", []string{"example.test:80", "192.0.2.1:80", "0.0.0.0:80", "127.0.0.1:0", "[::1]:65536", "127.0.0.1:abc", "127.0.0.1:+80", "[::1%lo]:80"}},
		{"--host", []string{"", "user@example.test", "https://example.test", "a b", "a\nb", "-a.test", "example.test:0", "[example.test]", "example.test:+80", strings.Repeat("a", 254)}},
		{"--target", []string{"", "http://example.test/", "path", "/a b", "/a\nb", "/a#fragment", "/" + strings.Repeat("a", 2048), "/%zz", "/a\u0085b", "/a\u00a0b", "/caf\u00e9", "/a\"b", "/a{b}|c"}},
		{"--timeout", []string{"0", "-1s", "11s", "bad"}}, {"--max-body", []string{"0", "1", "65537"}},
		{"--roots", []string{"/unused"}}, {"--sni", []string{"example.test"}},
	} {
		for _, value := range v.values {
			invalid = append(invalid, append(append([]string{}, good...), v.flag, value))
		}
	}
	for _, sni := range []string{"192.0.2.1", "bad_name.test", "-a.test", "a..test", strings.Repeat("a", 64) + ".test"} {
		invalid = append(invalid, append(append([]string{}, good...), "--tls", "--sni", sni))
	}
	t.Setenv("DOCKLET_PROBE_TEST", "")
	for i, args := range invalid {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			_, code, _ := invoke(t, args...)
			if code != 2 {
				t.Fatalf("usage code=%d args=%q", code, args)
			}
		})
	}
}

func TestTransportFailures(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	_ = l.Close()
	r, code, _ := invoke(t, cli(address)...)
	if code != 3 || r.Error != "connect-refused" || r.TLS != nil {
		t.Fatalf("refused: %+v code=%d", r, code)
	}
	for _, scenario := range []string{"timeout", "tls-timeout", "protocol", "body-read", "tls-handshake"} {
		t.Run(scenario, func(t *testing.T) {
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				c, e := l.Accept()
				if e != nil {
					return
				}
				defer c.Close()
				if scenario == "timeout" || scenario == "tls-timeout" {
					_, _ = io.Copy(io.Discard, c)
					return
				}
				b := make([]byte, 4096)
				_, _ = c.Read(b)
				switch scenario {
				case "protocol", "tls-handshake":
					_, _ = io.WriteString(c, "invalid\r\n\r\n")
				case "body-read":
					_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nx")
				}
			}()
			args := cli(l.Addr().String())
			if scenario == "timeout" || scenario == "tls-timeout" {
				args = append(args, "--timeout", "30ms")
			}
			if scenario == "tls-handshake" || scenario == "tls-timeout" {
				args = append(args, "--tls")
			}
			r, code, _ := invoke(t, args...)
			<-done
			expected := scenario
			if scenario == "tls-timeout" {
				expected = "timeout"
			}
			if code != 3 || r.Error != expected {
				t.Fatalf("transport: %+v code=%d want=%s", r, code, scenario)
			}
			if scenario == "tls-handshake" && (r.TLS == nil || r.TLS.Peer != nil) {
				t.Fatalf("failed handshake peer: %+v", r.TLS)
			}
		})
	}
}

func TestVersionAndSelftest(t *testing.T) {
	_, code, b := invoke(t, "version")
	if code != 0 || !bytes.Contains(b, []byte(`"version":"development"`)) {
		t.Fatalf("version: %s", b)
	}
	_, code, b = invoke(t, "selftest")
	if code != 0 || string(b) != "{\"v\":1,\"ok\":true}\n" {
		t.Fatalf("selftest: %s code=%d", b, code)
	}
}

func TestBoundedUTF8AndJSONWorstCase(t *testing.T) {
	s := bounded(strings.Repeat("é", 200)+"\x00", 255)
	if len(s) > 255 || !json.Valid([]byte(fmt.Sprintf("%q", s))) {
		t.Fatal("bounded UTF-8")
	}
	r := result{V: 1, TLS: &tlsResult{SNI: strings.Repeat("a", 253), Chain: []string{}, Peer: &peerResult{Subject: strings.Repeat(`"`, 256), Issuer: strings.Repeat(`\`, 256)}}}
	for range 32 {
		r.TLS.Peer.SANs = append(r.TLS.Peer.SANs, strings.Repeat(`"`, 256))
	}
	r.Headers = map[string]string{}
	for _, name := range []string{"location", "server", "content-type", "content-length", "www-authenticate"} {
		r.Headers[name] = strings.Repeat(`"`, 1024)
	}
	var out bytes.Buffer
	if emit(&out, r, 0) != 0 || out.Len() > 32768 || out.Len() == 0 {
		t.Fatalf("worst-case JSON: %d", out.Len())
	}
}

func TestEmptyHeadersRemainObject(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, e := l.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		b := make([]byte, 4096)
		_, _ = c.Read(b)
		_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nConnection: close\r\n\r\n")
	}()
	r, code, b := invoke(t, cli(l.Addr().String())...)
	<-done
	if code != 0 || r.Headers == nil || !bytes.Contains(b, []byte(`"headers":{}`)) {
		t.Fatalf("empty header object missing: %s", b)
	}
}
