// Source: docklet-next/issues/493
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original stdlib-only HTTP/TLS probe and its isolated contract tests.
// Copyright 2026 Docklet contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type authority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

func issue(t *testing.T, parent *authority, ca bool, start, end time.Time, names []string, subject string) authority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: subject}, DNSNames: names, NotBefore: start, NotAfter: end,
		IsCA: ca, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	if ca {
		template.KeyUsage |= x509.KeyUsageCertSign
	}
	signer, keySigner := template, key
	if parent != nil {
		signer, keySigner = parent.cert, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, signer, &key.PublicKey, keySigner)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return authority{cert, key}
}
func rootsFile(t *testing.T, root authority) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "roots.pem")
	if err := os.WriteFile(file, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root.cert.Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	return file
}
func tlsServer(t *testing.T, leaf authority, chain ...authority) *httptest.Server {
	t.Helper()
	cert := tls.Certificate{Certificate: [][]byte{leaf.cert.Raw}, PrivateKey: leaf.key}
	for _, a := range chain {
		cert.Certificate = append(cert.Certificate, a.cert.Raw)
	}
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "TLS body") }))
	s.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	s.StartTLS()
	t.Cleanup(s.Close)
	return s
}

func TestP3IndependentTLSVerification(t *testing.T) {
	t.Setenv("DOCKLET_PROBE_TEST", "1")
	now := time.Now()
	root := issue(t, nil, true, now.Add(-24*time.Hour), now.Add(24*time.Hour), nil, "root.example.test")
	other := issue(t, nil, true, now.Add(-24*time.Hour), now.Add(24*time.Hour), nil, "other.example.test")
	intermediate := issue(t, &root, true, now.Add(-24*time.Hour), now.Add(24*time.Hour), nil, "intermediate.example.test")
	for _, tc := range []struct {
		name         string
		trust, match bool
		validity     string
		start, end   time.Time
		roots        authority
		chain        bool
		sni          string
	}{
		{"trusted", true, true, "valid", now.Add(-time.Hour), now.Add(time.Hour), root, true, "example.test"},
		{"untrusted-matching", false, true, "valid", now.Add(-time.Hour), now.Add(time.Hour), other, true, "example.test"},
		{"wrong-name", true, false, "valid", now.Add(-time.Hour), now.Add(time.Hour), root, true, "wrong.example.test"},
		{"expired", false, true, "expired", now.Add(-2 * time.Hour), now.Add(-time.Hour), root, true, "example.test"},
		{"not-yet-valid", false, true, "not-yet-valid", now.Add(time.Hour), now.Add(2 * time.Hour), root, true, "example.test"},
		{"missing-intermediate", false, true, "valid", now.Add(-time.Hour), now.Add(time.Hour), root, false, "example.test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			leaf := issue(t, &intermediate, false, tc.start, tc.end, []string{"example.test"}, "example.test")
			var chain []authority
			if tc.chain {
				chain = []authority{intermediate}
			}
			server := tlsServer(t, leaf, chain...)
			args := append(cli(endpoint(server)), "--tls", "--sni", tc.sni, "--roots", rootsFile(t, tc.roots))
			r, code, _ := invoke(t, args...)
			if code != 0 || r.TLS == nil || r.TLS.Peer == nil || r.TLS.Version == "" {
				t.Fatalf("TLS answer: %+v code=%d", r, code)
			}
			v := r.TLS.Verify
			if v.ChainTrusted != tc.trust || v.HostnameMatch != tc.match || v.Validity != tc.validity {
				t.Fatalf("independent verification = %+v; want trusted=%v hostname=%v validity=%s", v, tc.trust, tc.match, tc.validity)
			}
			if (v.Error == "") != (tc.trust && tc.match) {
				t.Fatalf("verification error: %+v", v)
			}
			if r.TLS.Peer.SHA256 != hash(leaf.cert.Raw) || len(r.TLS.Chain) != 1+len(chain) || r.TLS.Chain[0] != r.TLS.Peer.SHA256 {
				t.Fatal("peer/chain fingerprint")
			}
		})
	}
}

func TestP10PeerBoundary(t *testing.T) {
	now := time.Now()
	root := issue(t, nil, true, now.Add(-time.Hour), now.Add(time.Hour), nil, strings.Repeat(`"`, 600))
	names := []string{"example.test"}
	for range 40 {
		names = append(names, strings.Repeat("a", 300)+".example.test")
	}
	leaf := issue(t, &root, false, now.Add(-time.Hour), now.Add(time.Hour), names, strings.Repeat(`\`, 600))
	server := tlsServer(t, leaf, root, root, root, root, root)
	r, code, _ := invoke(t, append(cli(endpoint(server)), "--tls")...)
	if code != 0 {
		t.Fatalf("code=%d result=%+v", code, r)
	}
	p := r.TLS.Peer
	if len(p.Subject) > 256 || len(p.Issuer) > 256 || len(p.SANs) != 32 || len(r.TLS.Chain) != 4 {
		t.Fatalf("peer limits: subject=%d issuer=%d SANs=%d chain=%d", len(p.Subject), len(p.Issuer), len(p.SANs), len(r.TLS.Chain))
	}
	for _, name := range p.SANs {
		if len(name) > 256 {
			t.Fatalf("SAN bytes=%d", len(name))
		}
	}
}

func TestRootsGate(t *testing.T) {
	args := append(cli("127.0.0.1:1"), "--tls", "--roots", "/unused")
	t.Setenv("DOCKLET_PROBE_TEST", "")
	_, code, _ := invoke(t, args...)
	if code != 2 {
		t.Fatal("roots accepted outside test mode")
	}
	t.Setenv("DOCKLET_PROBE_TEST", "1")
	_, code, _ = invoke(t, args...)
	if code != 2 {
		t.Fatal("unreadable roots accepted")
	}
}
