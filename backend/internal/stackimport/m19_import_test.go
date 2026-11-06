// Source: docklet-next/issues/348
// Revision: 8696c990fba68be0dd3ca9ac4de61434f6c56131
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original Docker Hub request budget and default stack name tests.

package stackimport

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	dto "github.com/florianfinn/docklet-next/types/stackimport"
	"github.com/google/go-containerregistry/pkg/name"
	registryHTTP "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The Docker Hub flow (HTTPS, anonymous bearer token, multi-arch index, config blob redirected to a CDN host)
// resolves within the request budget through the real resolver.
func TestM19RegistryImportDockerHubFlowFitsTheBudget(t *testing.T) {
	inner := httptest.NewServer(registryHTTP.New(registryHTTP.Logger(log.New(io.Discard, "", 0))))
	defer inner.Close()
	innerHost := strings.TrimPrefix(inner.URL, "http://")
	imageFor := func(arch string) v1.Image {
		img, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: arch, Config: v1.Config{ExposedPorts: map[string]struct{}{"80/tcp": {}}}})
		require.NoError(t, err)
		return img
	}
	index := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{Add: imageFor("amd64"), Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "amd64"}}},
		mutate.IndexAddendum{Add: imageFor("arm64"), Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "arm64"}}})
	tag, _ := name.NewTag(innerHost+"/team/app:1", name.Insecure)
	require.NoError(t, remote.WriteIndex(tag, index))
	var mu sync.Mutex
	var seen []string
	front := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.Host+r.URL.Path)
		mu.Unlock()
		switch {
		case r.Host == "cdn.example.com":
			proxyHubRequest(w, r, inner.URL+strings.TrimPrefix(r.URL.Path, "/cdn"))
		case r.URL.Path == "/token":
			_, _ = io.WriteString(w, `{"token":"example"}`)
		case r.Header.Get("Authorization") != "Bearer example":
			w.Header().Set("WWW-Authenticate", `Bearer realm="https://registry.example.com/token",service="registry.example.com",scope="repository:team/app:pull"`)
			w.WriteHeader(401)
		case strings.Contains(r.URL.Path, "/blobs/"):
			http.Redirect(w, r, "https://cdn.example.com/cdn"+r.URL.Path, http.StatusTemporaryRedirect)
		default:
			proxyHubRequest(w, r, inner.URL+r.URL.Path)
		}
	}))
	defer front.Close()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // test TLS server
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, front.Listener.Addr().String())
	}
	gdb, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	_ = gdb.AutoMigrate(&registry.ContainerRegistry{})
	_ = gdb.Create(&registry.ContainerRegistry{URL: "https://registry.example.com", Enabled: true}).Error
	svc := registry.NewContainerRegistryService(&database.DB{DB: gdb}, nil, nil, nil, &http.Client{Transport: transport})
	resolver, err := NewResolver(svc, "")
	require.NoError(t, err)
	result, err := resolver.Resolve(t.Context(), dto.Request{Kind: "registry", Ref: "registry.example.com/team/app:1", Platform: &dto.Platform{OS: "linux", Architecture: "arm64"}})
	require.NoError(t, err, "requests %q", seen)
	require.Equal(t, "app", result.Title)
	// Ping, token, index, platform manifest, config blob and its CDN redirect use the whole budget.
	require.Len(t, seen, requestLimit)
}

// Default stack names meet the stack name rule and the 63-character exclusion limit.
func TestM19StackTitleMeetsTheStackNameRule(t *testing.T) {
	rule := regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	for in, want := range map[string]string{"My.Repo": "my-repo", "_x": "x", "-x-": "x", "...": "app", "---": "app", "9app": "9app", "": "app", strings.Repeat("a", 62) + "_b": strings.Repeat("a", 62), strings.Repeat("x", 100): strings.Repeat("x", 63)} {
		got := stackTitle(in)
		require.Equal(t, want, got, in)
		require.Regexp(t, rule, got)
		require.LessOrEqual(t, len(got), 63)
	}
}

func proxyHubRequest(w http.ResponseWriter, r *http.Request, target string) {
	req, _ := http.NewRequestWithContext(r.Context(), r.Method, target, nil)
	req.Header = r.Header.Clone()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		w.WriteHeader(502)
		return
	}
	defer res.Body.Close()
	for k, v := range res.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
}
