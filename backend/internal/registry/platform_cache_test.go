// Source: docklet-next/issues/341
// Revision: 5d46f0259a31fe910e9bf1ad145398e95eb3a77c
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Unchanged images are observed by HEAD and do not spend registry pulls.
package registry

import (
	"bytes"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	registryHTTP "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"
)

func TestPlatformImageSpendsNoPullOnUnchangedImage(t *testing.T) {
	var manifestGets atomic.Int32
	inner := registryHTTP.New(registryHTTP.Logger(log.New(io.Discard, "", 0)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/manifests/") {
			manifestGets.Add(1)
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	repository := strings.TrimPrefix(server.URL, "http://") + "/app"
	publish := func(version string) {
		img, e := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64", Config: v1.Config{Labels: map[string]string{"org.opencontainers.image.version": version}}})
		require.NoError(t, e)
		tag, e := name.NewTag(repository+":stable", name.Insecure)
		require.NoError(t, e)
		require.NoError(t, remote.Write(tag, img, remote.WithContext(t.Context())))
	}
	db := setupContainerRegistryTestDBInternal(t)
	require.NoError(t, db.Create(&ContainerRegistry{URL: server.URL, RegistryType: "generic", Insecure: true, Enabled: true}).Error)
	s := NewContainerRegistryService(db, nil, nil, nil)
	platform := v1.Platform{OS: "linux", Architecture: "amd64"}

	publish("1.0.0")
	first, err := s.PlatformImage(t.Context(), repository+":stable", platform)
	require.NoError(t, err)
	require.Equal(t, "1.0.0", first.Version)
	spent := manifestGets.Load()
	again, err := s.PlatformImage(t.Context(), repository+":stable", platform)
	require.NoError(t, err)
	require.Equal(t, first, again)
	require.Equal(t, spent, manifestGets.Load(), "an unchanged image must not download its manifest again")

	publish("1.1.0")
	moved, err := s.PlatformImage(t.Context(), repository+":stable", platform)
	require.NoError(t, err)
	require.Equal(t, "1.1.0", moved.Version)
	require.NotEqual(t, first.Digest, moved.Digest)
	require.Greater(t, manifestGets.Load(), spent, "a moved tag must be fetched")
}

func TestPlatformImageLogsRateLimitCause(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	var heads, gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodHead {
			heads.Add(1)
		} else {
			gets.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"errors":[{"code":"TOOMANYREQUESTS","message":"synthetic limit"}]}`))
	}))
	t.Cleanup(server.Close)
	db := setupContainerRegistryTestDBInternal(t)
	require.NoError(t, db.Create(&ContainerRegistry{URL: server.URL, RegistryType: "generic", Insecure: true, Enabled: true}).Error)
	s := NewContainerRegistryService(db, nil, nil, nil)
	_, err := s.PlatformImage(t.Context(), strings.TrimPrefix(server.URL, "http://")+"/app:1.0.0", v1.Platform{OS: "linux", Architecture: "amd64"})
	require.Error(t, err)
	require.Contains(t, logs.String(), "Registry platform lookup failed")
	require.Contains(t, logs.String(), "code=registry.rate-limited")
	require.Contains(t, logs.String(), "status=429")
	require.NotContains(t, logs.String(), "synthetic limit")
	require.Equal(t, int32(1), heads.Load(), "a rate-limited HEAD is tried once")
	require.Zero(t, gets.Load(), "a rate-limited HEAD spends no manifest download")
}
