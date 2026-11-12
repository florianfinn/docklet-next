// Source: docklet-next/issues/438
// Revision: 57e835f7cce5778fd3d164298e5b488e11abb71d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original read-only candidate resolver accessor preserving Retry-After.

package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/distribution/reference"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type candidateTransport struct {
	base  http.RoundTripper
	mu    sync.Mutex
	retry time.Time
}

func (t *candidateTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// The platform resolver can retry a GET after its HEAD fallback.
	t.mu.Lock()
	retry := t.retry
	t.mu.Unlock()
	if retry.After(time.Now()) {
		return &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {retry.UTC().Format(http.TimeFormat)}}, Body: http.NoBody, Request: req}, nil
	}
	response, err := t.base.RoundTrip(req)
	if err == nil && response.StatusCode == http.StatusTooManyRequests {
		retry := time.Now().Add(time.Minute)
		value := response.Header.Get("Retry-After")
		if seconds, e := strconv.ParseInt(value, 10, 32); e == nil && seconds >= 0 {
			retry = time.Now().Add(time.Duration(seconds) * time.Second)
		} else if date, e := http.ParseTime(value); e == nil {
			retry = date
		}
		t.mu.Lock()
		if retry.After(t.retry) {
			t.retry = retry
		}
		t.mu.Unlock()
	}
	return response, err
}

// CandidatePlatformImage uses the ordinary platform resolver and credentials.
// A per-call client copy observes only retry headers, never registry diagnostics.
func (s *ContainerRegistryService) CandidatePlatformImage(ctx context.Context, ref string, platform v1.Platform) (PlatformObservation, time.Time, error) {
	if retry := s.candidateRetryAt(ctx, ref); retry.After(time.Now()) {
		return PlatformObservation{}, retry, imageDTO.Failure("registry.rate-limited", 429)
	}
	copy, transport := s.candidateClient()
	out, err := copy.PlatformImage(ctx, ref, platform)
	retry := s.candidateRecordRetry(ctx, ref, transport)
	return out, retry, err
}

func (s *ContainerRegistryService) CandidateImageTags(ctx context.Context, ref string) ([]string, time.Time, error) {
	if retry := s.candidateRetryAt(ctx, ref); retry.After(time.Now()) {
		return nil, retry, imageDTO.Failure("registry.rate-limited", 429)
	}
	copy, transport := s.candidateClient()
	parsed, options, auth, err := copy.platformOptionsInternal(ctx, ref, v1.Platform{OS: "linux", Architecture: "amd64"}, false)
	if err != nil {
		return nil, time.Time{}, err
	}
	tags, err := remote.List(parsed.Context(), append(options, remote.WithRetryBackoff(remote.Backoff{Steps: 1}))...)
	retry := s.candidateRecordRetry(ctx, ref, transport)
	if err != nil {
		err = registryProbeError(err, auth == "anonymous")
	}
	return tags, retry, err
}

func (s *ContainerRegistryService) candidateClient() (*ContainerRegistryService, *candidateTransport) {
	client := *s.distributionHTTPClient
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	transport := &candidateTransport{base: base}
	client.Transport = transport
	copy := &ContainerRegistryService{db: s.db, dockerClient: s.dockerClient, cache: s.cache, labelCache: s.labelCache, kvService: s.kvService, settingsService: s.settingsService, distributionHTTPClient: &client}
	return copy, transport
}

func candidateRetryKey(raw string) string {
	named, err := reference.ParseNormalizedNamed(raw)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256([]byte(reference.Domain(named)))
	return "candidate:registry-retry:" + hex.EncodeToString(hash[:])
}
func (s *ContainerRegistryService) candidateRetryAt(ctx context.Context, ref string) time.Time {
	if s.kvService == nil {
		return time.Time{}
	}
	raw, ok, err := s.kvService.Get(ctx, candidateRetryKey(ref))
	if err != nil || !ok {
		return time.Time{}
	}
	stamp, _ := time.Parse(time.RFC3339Nano, raw)
	return stamp
}
func (s *ContainerRegistryService) candidateRecordRetry(ctx context.Context, ref string, transport *candidateTransport) time.Time {
	transport.mu.Lock()
	retry := transport.retry
	transport.mu.Unlock()
	if !retry.IsZero() && s.kvService != nil {
		_ = s.kvService.Set(ctx, candidateRetryKey(ref), retry.UTC().Format(time.RFC3339Nano))
	}
	return retry
}
