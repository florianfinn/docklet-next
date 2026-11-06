// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package registry

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/stackimport"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	registryHTTP "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"
)

func importWrap(rt http.RoundTripper) http.RoundTripper { return rt }
func requireImportError(t *testing.T, err error, code string) {
	t.Helper()
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, code, p.Code)
	require.Equal(t, code, err.Error())
}
func TestImportUnconfiguredRegistryNoRequest(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	db := setupContainerRegistryTestDBInternal(t)
	svc := NewContainerRegistryService(db, nil, nil, nil)
	raw := strings.TrimPrefix(server.URL, "http://") + "/app:1"
	for _, configured := range []bool{false, true} {
		if configured {
			require.NoError(t, db.Create(&ContainerRegistry{URL: server.URL, Enabled: false, Insecure: true}).Error)
		}
		_, err := svc.ResolveImportImage(t.Context(), raw, v1.Platform{OS: "linux", Architecture: "amd64"}, importWrap)
		requireImportError(t, err, "import.registry-not-configured")
	}
	require.Zero(t, calls.Load())
}

func TestImportRegistryTransportAuthInsecureAndPlatform(t *testing.T) {
	inner := registryHTTP.New(registryHTTP.Logger(log.New(io.Discard, "", 0)))
	var authenticated atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "test-user" || password != "test-only" {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(401)
			return
		}
		authenticated.Add(1)
		inner.ServeHTTP(w, r)
	}))
	defer server.Close()
	// An explicit public synthetic hostname requires the configured insecure flag.
	// DialContext still uses a real HTTP transport and the actual httptest receiver.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	defer transport.CloseIdleConnections()
	repo := "registry.example.com/team/app"
	tag, err := name.NewTag(repo+":stable", name.Insecure)
	require.NoError(t, err)
	imageFor := func(arch, version string) v1.Image {
		img, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: arch, Config: v1.Config{Labels: map[string]string{"org.opencontainers.image.version": version}, Env: []string{"TEST_DEFAULT=ok", "TEST_REQUIRED"}}})
		require.NoError(t, err)
		return img
	}
	amd, arm := imageFor("amd64", "amd"), imageFor("arm64", "arm")
	index := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: amd, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "amd64"}}}, mutate.IndexAddendum{Add: arm, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "arm64"}}})
	require.NoError(t, remote.WriteIndex(tag, index, remote.WithTransport(transport), remote.WithAuth(authn.FromConfig(authn.AuthConfig{Username: "test-user", Password: "test-only"}))))
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "http://registry.example.com", "test-user", "test-only")
	require.NoError(t, db.Model(&ContainerRegistry{}).Where("url = ?", "http://registry.example.com").Update("insecure", true).Error)
	svc := NewContainerRegistryService(db, nil, nil, nil, &http.Client{Transport: transport})
	for _, arch := range []string{"amd64", "arm64"} {
		platform := v1.Platform{OS: "linux", Architecture: arch}
		result, err := svc.ResolveImportImage(t.Context(), tag.Name(), platform, importWrap)
		require.NoError(t, err)
		require.Equal(t, arch, result.Config.Architecture)
		expected := amd
		if arch == "arm64" {
			expected = arm
		}
		digest, err := expected.Digest()
		require.NoError(t, err)
		require.Equal(t, digest.String(), result.Digest)
		old, err := svc.PlatformImage(t.Context(), tag.Name(), platform)
		require.NoError(t, err)
		require.Equal(t, result.Digest, old.Digest)
		require.Equal(t, "credential", old.AuthState)
	}
	require.Positive(t, authenticated.Load())
	require.NoError(t, db.Model(&ContainerRegistry{}).Where("url = ?", "http://registry.example.com").Update("insecure", false).Error)
	_, err = svc.ResolveImportImage(t.Context(), tag.Name(), v1.Platform{OS: "linux", Architecture: "amd64"}, importWrap)
	requireImportError(t, err, "import.source-unavailable")
}

func TestImportConfigByteBound(t *testing.T) {
	for _, mode := range []string{"descriptor", "embedded", "embedded-understated", "blob-understated"} {
		t.Run(mode, func(t *testing.T) {
			// Matching digest ensures removing the cap accepts the hostile config.
			cfg := []byte(`{"os":"linux","architecture":"amd64","config":{"Env":["PADDING=` + strings.Repeat("x", 2<<20) + `"]}}`)
			digest, size, err := v1.SHA256(bytes.NewReader(cfg))
			require.NoError(t, err)
			descriptor := v1.Descriptor{MediaType: types.OCIConfigJSON, Digest: digest, Size: size}
			if strings.HasPrefix(mode, "embedded") {
				descriptor.Data = cfg
			}
			if strings.HasSuffix(mode, "understated") {
				descriptor.Size = 1
			}
			manifest, err := json.Marshal(v1.Manifest{SchemaVersion: 2, MediaType: types.OCIManifestSchema1, Config: descriptor, Layers: []v1.Descriptor{}})
			require.NoError(t, err)
			var blobGets atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v2/":
				case strings.Contains(r.URL.Path, "/manifests/"):
					w.Header().Set("Content-Type", string(types.OCIManifestSchema1))
					_, _ = w.Write(manifest)
				case strings.Contains(r.URL.Path, "/blobs/"):
					blobGets.Add(1)
					_, _ = w.Write(cfg)
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			db := setupContainerRegistryTestDBInternal(t)
			require.NoError(t, db.Create(&ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true}).Error)
			svc := NewContainerRegistryService(db, nil, nil, nil)
			_, err = svc.ResolveImportImage(t.Context(), strings.TrimPrefix(server.URL, "http://")+"/app:1", v1.Platform{OS: "linux", Architecture: "amd64"}, importWrap)
			requireImportError(t, err, "import.too-large")
			if mode != "blob-understated" {
				require.Zero(t, blobGets.Load(), "reject the descriptor before fetching config")
			} else {
				require.Equal(t, int32(1), blobGets.Load())
			}
		})
	}
}
func TestImportRegistryNeutralStatus(t *testing.T) {
	for status, code := range map[int]string{401: "import.unauthorized", 403: "import.unauthorized", 404: "import.not-found", 500: "import.source-unavailable"} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v2/" {
					return
				}
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "upstream secret body")
			}))
			defer server.Close()
			db := setupContainerRegistryTestDBInternal(t)
			require.NoError(t, db.Create(&ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true}).Error)
			svc := NewContainerRegistryService(db, nil, nil, nil)
			_, err := svc.ResolveImportImage(t.Context(), strings.TrimPrefix(server.URL, "http://")+"/app:1", v1.Platform{OS: "linux", Architecture: "amd64"}, importWrap)
			requireImportError(t, err, code)
		})
	}
}
