// Source: docklet-next/issues/332
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original bounded widget transport and certificate pin verification.

package widget

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	dto "github.com/florianfinn/docklet-next/types/widget"
)

const fetchBudget = 5 * time.Second
const bodyLimit = 1 << 20

type fetchClient struct {
	client     *http.Client
	base       *url.URL
	credential string
	kind       dto.Kind
}

func newFetchClient(c dto.Config, credential string) *fetchClient {
	u, _ := url.Parse(c.Address)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// LAN targets are intentional; operator proxy environment variables must not
	// redirect credentials to a different authority.
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	transport.TLSClientConfig.InsecureSkipVerify = true // Verification below always requires a valid chain or an exact authority-bound pin.
	transport.TLSClientConfig.VerifyConnection = func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return dto.Failure(dto.TLSFailed)
		}
		sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
		observed := &dto.CertificatePin{SHA256: hex.EncodeToString(sum[:]), HostPort: authority(u)}
		failure := &dto.Problem{Code: dto.TLSFailed, Certificate: observed}
		if c.TLSPin != nil {
			expected, err := hex.DecodeString(c.TLSPin.SHA256)
			if err != nil || c.TLSPin.HostPort != authority(u) || subtle.ConstantTimeCompare(expected, sum[:]) != 1 {
				return failure
			}
			return nil
		}
		intermediates := x509.NewCertPool()
		for _, cert := range cs.PeerCertificates[1:] {
			intermediates.AddCert(cert)
		}
		if _, err := cs.PeerCertificates[0].Verify(x509.VerifyOptions{DNSName: u.Hostname(), Intermediates: intermediates}); err != nil {
			return failure
		}
		return nil
	}
	client := &http.Client{Transport: transport, Timeout: fetchBudget}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Scheme != u.Scheme || authority(req.URL) != authority(u) || req.URL.User != nil {
			return dto.Failure(dto.RedirectRefused)
		}
		return nil
	}
	return &fetchClient{client: client, base: u, credential: credential, kind: c.Kind}
}

func (f *fetchClient) close() { f.client.CloseIdleConnections() }

func (f *fetchClient) request(ctx context.Context, method, path string, out any) error {
	u := *f.base
	u.Path = strings.TrimRight(u.Path, "/") + path
	// Driver paths are constants except for decimal IDs and trusted media enums.
	if before, query, found := strings.Cut(u.Path, "?"); found {
		u.Path = before
		u.RawQuery = query
	}
	u.RawPath = ""
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return dto.Failure(dto.InvalidConfig)
	}
	req.Header.Set("Accept", "application/json")
	if f.kind == dto.Plex {
		req.Header.Set("X-Plex-Token", f.credential)
	} else {
		req.Header.Set("X-Api-Key", f.credential)
	}
	response, err := f.client.Do(req)
	if err != nil {
		return transportProblem(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return dto.Failure(dto.AuthFailed)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return dto.Failure(dto.BadResponse)
	}
	b, err := io.ReadAll(io.LimitReader(response.Body, bodyLimit+1))
	if err != nil {
		return transportProblem(ctx, err)
	}
	if len(b) > bodyLimit {
		return dto.Failure(dto.BadResponse)
	}
	if out != nil && json.Unmarshal(b, out) != nil {
		return dto.Failure(dto.BadResponse)
	}
	return nil
}

func transportProblem(ctx context.Context, err error) *dto.Problem {
	var p *dto.Problem
	if errors.As(err, &p) {
		return p
	}
	var netErr net.Error
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return dto.Failure(dto.Timeout)
	}
	var certErr *tls.CertificateVerificationError
	var recordErr tls.RecordHeaderError
	if errors.As(err, &certErr) || errors.As(err, &recordErr) {
		return dto.Failure(dto.TLSFailed)
	}
	return dto.Failure(dto.Unreachable)
}

func problem(err error) *dto.Problem {
	var p *dto.Problem
	if errors.As(err, &p) {
		copy := *p
		return &copy
	}
	return dto.Failure(dto.Unreachable)
}
