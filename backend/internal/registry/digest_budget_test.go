// Source: docklet-next/issues/677
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original regression proofs.

package registry

import (
	"context"
	"errors"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDigestDaemonReservesFallbackBudget(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	parentDeadline, _ := ctx.Deadline()
	svc := NewContainerRegistryService(nil, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{distributionInspectFn: func(child context.Context, _ string, _ client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
			deadline, ok := child.Deadline()
			require.True(t, ok)
			require.Greater(t, parentDeadline.Sub(deadline), 900*time.Millisecond)
			return client.DistributionInspectResult{}, context.DeadlineExceeded
		}}, nil
	}, nil, nil)
	called := false
	svc.distributionHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.NoError(t, req.Context().Err())
		called = true
		header := http.Header{"Content-Type": []string{"application/vnd.docker.distribution.manifest.v2+json"}, "Docker-Content-Digest": []string{"sha256:" + strings.Repeat("a", 64)}}
		return &http.Response{StatusCode: 200, Header: header, Body: http.NoBody}, nil
	})}
	result, err := svc.InspectImageDigest(ctx, "registry.example.com/app:1", nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, called)
}
func TestTLSHandshakeTimeoutEligibleButTrustPermanent(t *testing.T) {
	require.True(t, isDistributionFallbackEligibleInternal(errors.New("net/http: TLS handshake timeout")))
	for _, text := range []string{"x509: certificate signed by unknown authority", "tls: failed to verify certificate", "tls: protocol error"} {
		require.False(t, isDistributionFallbackEligibleInternal(errors.New(text)), text)
	}
}
