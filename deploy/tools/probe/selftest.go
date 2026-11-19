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
	"io"
	"math/big"
	"net"
	"net/http"
	"time"
)

// The selftest needs only loopback: it works in a container with --network none.
func selftest(out io.Writer) int {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return emit(out, result{V: 1, Error: "selftest"}, 3)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "example.test"}, DNSNames: []string{"example.test"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return emit(out, result{V: 1, Error: "selftest"}, 3)
	}
	for _, secure := range []bool{false, true} {
		listener, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			return emit(out, result{V: 1, Error: "selftest"}, 3)
		}
		connect := listener.Addr().String()
		if secure {
			listener = tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}})
		}
		server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "docklet-probe") }), ReadHeaderTimeout: time.Second}
		done := make(chan struct{})
		go func() { _ = server.Serve(listener); close(done) }()
		r, code := probe(options{connect: connect, host: "example.test", target: "/", sni: "example.test", tls: secure, timeout: time.Second, maxBody: bodyLimit})
		_ = server.Close()
		<-done
		if code != 0 || r.Status == nil || *r.Status != 200 || r.Body.SHA256 != hash([]byte("docklet-probe")) || secure && (r.TLS.Peer == nil || !r.TLS.Verify.HostnameMatch) {
			return emit(out, result{V: 1, Error: "selftest"}, 3)
		}
	}
	return emit(out, struct {
		V  int  `json:"v"`
		OK bool `json:"ok"`
	}{1, true}, 0)
}
