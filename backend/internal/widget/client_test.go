// Source: docklet-next/issues/332
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original real-transport egress and certificate pin regression tests.

package widget

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dto "github.com/florianfinn/docklet-next/types/widget"
	"github.com/stretchr/testify/require"
)

func TestSW2HTTPBodyCapBeforeDecode(t *testing.T) {
	var sent atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"MediaContainer":{"size":0},"padding":"`)
		chunk := strings.Repeat("a", 8192)
		for range 4096 {
			n, err := io.WriteString(w, chunk)
			sent.Add(int64(n))
			if err != nil {
				return
			}
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()
	f := newFetchClient(dto.Config{Kind: dto.Plex, Address: server.URL}, "")
	defer f.close()
	_, err := fetchPlex(t.Context(), f)
	require.Equal(t, dto.BadResponse, problem(err).Code)
	require.Less(t, sent.Load(), int64(32<<20), "stop consuming the chunked response after the cap")
}

func TestSW2RedirectBoundaries(t *testing.T) {
	var foreignCalls atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { foreignCalls.Add(1); respond(w, plexBody("Foreign")) }))
	defer foreign.Close()
	for _, mode := range []string{"foreign-port", "foreign-host", "scheme", "loop", "three"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "foreign-port":
					http.Redirect(w, r, foreign.URL, 302)
				case "foreign-host":
					http.Redirect(w, r, "http://other.example.com/status/sessions", 302)
				case "scheme":
					http.Redirect(w, r, "https://"+r.Host+r.URL.Path, 302)
				case "loop":
					http.Redirect(w, r, r.URL.Path, 302)
				case "three":
					n, _ := strconv.Atoi(r.URL.Query().Get("n"))
					if n < 3 {
						http.Redirect(w, r, "/status/sessions?n="+strconv.Itoa(n+1), 302)
					} else {
						respond(w, plexBody("Same authority"))
					}
				}
			}))
			defer server.Close()
			f := newFetchClient(dto.Config{Kind: dto.Plex, Address: server.URL}, "secret")
			defer f.close()
			out, err := fetchPlex(t.Context(), f)
			if mode == "three" {
				require.NoError(t, err)
				require.Equal(t, "Same authority", out.Sessions[0].Title)
			} else {
				require.Equal(t, dto.RedirectRefused, problem(err).Code)
			}
		})
	}
	require.Zero(t, foreignCalls.Load(), "credentials never reach a refused authority")
}

func TestSW2FiveSecondTotalOverseerrBudget(t *testing.T) {
	// Two individual calls fit under five seconds, but their combined duration
	// does not. The second sender observes cancellation through the real client.
	canceled := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/count") {
			select {
			case <-time.After(3 * time.Second):
				respond(w, `{"pending":0,"approved":0,"available":0}`)
			case <-r.Context().Done():
			}
			return
		}
		select {
		case <-time.After(3 * time.Second):
			respond(w, `{"results":[]}`)
		case <-r.Context().Done():
			canceled <- struct{}{}
		}
	}))
	defer server.Close()
	s := testService(t)
	configure(t, s, dto.Overseerr, server.URL)
	start := time.Now()
	result := s.Get(t.Context(), dto.Overseerr)
	require.NotNil(t, result.Error)
	require.Equal(t, dto.Timeout, result.Error.Code)
	require.Less(t, time.Since(start), 6*time.Second)
	wait(t, canceled)
	s.mu.Lock()
	cached := s.cache[dto.Overseerr]
	s.mu.Unlock()
	require.Equal(t, 1, cached.Failures, "timeout enters backoff too")
}

func certificate(t *testing.T, serial int64) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "service.example.com"}, DNSNames: []string{"service.example.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func TestSW15CertificatePinAuthorityAndRotation(t *testing.T) {
	var active atomic.Pointer[tls.Certificate]
	first := certificate(t, 1)
	second := certificate(t, 2)
	active.Store(&first)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { respond(w, plexBody("Pinned")) }))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
		return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{*active.Load()}}, nil
	}}
	server.StartTLS()
	defer server.Close()
	c := dto.Config{Kind: dto.Plex, Address: server.URL}
	get := func(c dto.Config) (*dto.PlexData, error) {
		f := newFetchClient(c, "")
		defer f.close()
		return fetchPlex(t.Context(), f)
	}
	_, err := get(c)
	p := problem(err)
	require.Equal(t, dto.TLSFailed, p.Code)
	require.NotNil(t, p.Certificate)
	sum := sha256.Sum256(first.Certificate[0])
	require.Equal(t, hex.EncodeToString(sum[:]), p.Certificate.SHA256)
	u, _ := url.Parse(server.URL)
	require.Equal(t, authority(u), p.Certificate.HostPort)
	c.TLSPin = p.Certificate
	result, err := get(c)
	require.NoError(t, err)
	require.Equal(t, "Pinned", result.Sessions[0].Title)
	active.Store(&second)
	_, err = get(c)
	require.Equal(t, dto.TLSFailed, problem(err).Code, "replaced leaf must fail even with a saved pin")
	active.Store(&first)
	foreign := *c.TLSPin
	foreign.HostPort = "other.example.com:443"
	c.TLSPin = &foreign
	_, err = get(c)
	require.Equal(t, dto.TLSFailed, problem(err).Code, "a pin must not trust the same certificate at a different authority")
	_, err = updatedRow(dto.Plex, row{}, dto.ConfigUpdate{Enabled: true, Address: server.URL, TLSPin: &foreign}, time.Now())
	require.Equal(t, dto.InvalidConfig, problem(err).Code)
	s := testService(t)
	tested, testErr := s.Test(t.Context(), dto.Plex, dto.ConfigUpdate{Enabled: true, Address: server.URL})
	require.NoError(t, testErr)
	require.Equal(t, dto.TLSFailed, tested.Error.Code)
	require.NotNil(t, tested.Error.Certificate)
	configured, saveErr := s.Save(t.Context(), dto.Plex, dto.ConfigUpdate{Enabled: true, Address: server.URL, TLSPin: tested.Error.Certificate})
	require.NoError(t, saveErr)
	require.NotNil(t, configured.TLSPin)
	require.Nil(t, s.Get(t.Context(), dto.Plex).Error)
	active.Store(&second)
	changed, saveErr := s.Save(t.Context(), dto.Plex, dto.ConfigUpdate{Enabled: true, Address: server.URL, TLSPin: configured.TLSPin})
	require.NoError(t, saveErr)
	require.Equal(t, configured.Revision+1, changed.Revision)
	live := s.Get(t.Context(), dto.Plex)
	require.Equal(t, dto.TLSFailed, live.Error.Code)
	require.Nil(t, live.Error.Certificate, "fingerprints are administrator-test output only")
}

func TestHTTPFailuresStayNeutral(t *testing.T) {
	for _, status := range []int{401, 403, 404, 500} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, "synthetic-private-token")
		}))
		f := newFetchClient(dto.Config{Kind: dto.Plex, Address: server.URL}, "synthetic-private-token")
		_, err := fetchPlex(t.Context(), f)
		want := dto.BadResponse
		if status == 401 || status == 403 {
			want = dto.AuthFailed
		}
		require.Equal(t, want, problem(err).Code)
		require.NotContains(t, err.Error(), "synthetic-private-token")
		f.close()
		server.Close()
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.Equal(t, dto.Unreachable, transportProblem(ctx, context.Canceled).Code)
}

// SW2 boundary: three same-authority redirects are followed, a fourth is refused.
func TestSW2FourthRedirectRefused(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		if n < 4 {
			http.Redirect(w, r, "/status/sessions?n="+strconv.Itoa(n+1), http.StatusFound)
			return
		}
		respond(w, plexBody("Four"))
	}))
	defer server.Close()
	f := newFetchClient(dto.Config{Kind: dto.Plex, Address: server.URL}, "")
	defer f.close()
	_, err := fetchPlex(t.Context(), f)
	require.Equal(t, dto.RedirectRefused, problem(err).Code)
	require.EqualValues(t, 4, hits.Load())
}
