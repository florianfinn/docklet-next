// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package stackimport

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/loader"
	compose "github.com/compose-spec/compose-go/v2/types"
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
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

func TestRegistryImportProposalRealTransport(t *testing.T) {
	inner := registryHTTP.New(registryHTTP.Logger(log.New(io.Discard, "", 0)))
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); inner.ServeHTTP(w, r) }))
	defer server.Close()
	repo := strings.TrimPrefix(server.URL, "http://") + "/team/app"
	cfg := &v1.ConfigFile{OS: runtime.GOOS, Architecture: runtime.GOARCH, Config: v1.Config{ExposedPorts: map[string]struct{}{"8080/tcp": {}, "5353/udp": {}}, Volumes: map[string]struct{}{"/var/lib/app": {}, "/var/lib/other": {}}, Env: []string{"HAS_DEFAULT=value", "REQUIRED", "EMPTY=", "INVALID-KEY=bad", "LITERAL=$UNCHANGED"}}}
	img, err := mutate.ConfigFile(empty.Image, cfg)
	require.NoError(t, err)
	tag, err := name.NewTag(repo+":latest", name.Insecure)
	require.NoError(t, err)
	require.NoError(t, remote.Write(tag, img))
	gdb, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(&registry.ContainerRegistry{}))
	require.NoError(t, gdb.Create(&registry.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true}).Error)
	svc := registry.NewContainerRegistryService(&database.DB{DB: gdb}, nil, nil, nil)
	resolver, err := NewResolver(svc, "")
	require.NoError(t, err)
	calls.Store(0)
	result, err := resolver.Resolve(t.Context(), dto.Request{Kind: "registry", Ref: repo})
	require.NoError(t, err)
	require.LessOrEqual(t, calls.Load(), int32(6))
	require.Positive(t, calls.Load())
	require.Equal(t, "app", result.Title)
	require.Equal(t, repo+":latest", result.Source.CanonicalRef)
	digest, err := img.Digest()
	require.NoError(t, err)
	require.Equal(t, digest.String(), result.Source.ResolvedDigest)
	require.Equal(t, []string{"REQUIRED"}, result.EnvNames)
	require.Equal(t, []string{"import.found.image-config", "import.found.ports", "import.found.volumes", "import.found.environment"}, result.Found)
	require.Equal(t, []string{"import.missing.compose"}, result.Missing)
	var model map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(result.ComposeContent), &model))
	service := model["services"].(map[string]any)["app"].(map[string]any)
	require.Equal(t, repo+":latest", service["image"])
	require.Equal(t, "unless-stopped", service["restart"])
	require.Equal(t, []any{"5353:5353/udp", "8080:8080/tcp"}, service["ports"])
	require.Equal(t, []any{"app-data-1:/var/lib/app", "app-data-2:/var/lib/other"}, service["volumes"])
	require.Equal(t, map[string]any{"HAS_DEFAULT": "value", "REQUIRED": nil, "EMPTY": "", "LITERAL": "$$UNCHANGED"}, service["environment"])
	proposal, err := parseProposal(t.Context(), []byte(result.ComposeContent))
	require.NoError(t, err)
	require.Equal(t, result.ComposeContent, proposal.ComposeContent)
	require.NotContains(t, proposal.EnvNames, "UNCHANGED")
	deployed, err := loader.LoadWithContext(t.Context(), compose.ConfigDetails{Environment: compose.Mapping{"UNCHANGED": "unexpected"}, ConfigFiles: []compose.ConfigFile{{Filename: "proposal.yaml", Content: []byte(result.ComposeContent)}}}, func(o *loader.Options) {
		o.SetProjectName("proposal", true)
		o.ResolvePaths = false
		o.SkipResolveEnvironment = true
	})
	require.NoError(t, err)
	literal := deployed.Services["app"].Environment["LITERAL"]
	require.NotNil(t, literal)
	require.Equal(t, "$UNCHANGED", *literal, "image defaults must remain literal through the deployment parser")
	for _, raw := range []string{"https://user:example@registry.example.com/app", "registry.example.com/app?token=example", "UPPER/Bad", " app "} {
		calls.Store(0)
		_, err := resolver.Resolve(t.Context(), dto.Request{Kind: "registry", Ref: raw})
		requireProblem(t, err, "import.invalid-reference", 400)
		require.Zero(t, calls.Load())
	}
	for _, bad := range []string{"invalid-port", "large-proposal"} {
		t.Run(bad, func(t *testing.T) {
			changed := *cfg
			if bad == "invalid-port" {
				changed.Config.ExposedPorts = map[string]struct{}{"invalid/tcp": {}}
			} else {
				changed.Config.Env = []string{"PADDING=" + strings.Repeat("x", fileLimit)}
			}
			image, err := mutate.ConfigFile(empty.Image, &changed)
			require.NoError(t, err)
			require.NoError(t, remote.Write(tag, image))
			_, err = resolver.Resolve(t.Context(), dto.Request{Kind: "registry", Ref: repo})
			if bad == "invalid-port" {
				requireProblem(t, err, "import.invalid-compose", 422)
			} else {
				requireProblem(t, err, "import.too-large", 413)
			}
		})
	}
}

func TestRegistryImportDockerHubAndOriginalImageRef(t *testing.T) {
	server := httptest.NewServer(registryHTTP.New(registryHTTP.Logger(log.New(io.Discard, "", 0))))
	defer server.Close()
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	local := strings.TrimPrefix(server.URL, "http://") + "/library/nginx:latest"
	tag, err := name.NewTag(local, name.Insecure)
	require.NoError(t, err)
	img, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: runtime.GOOS, Architecture: runtime.GOARCH})
	require.NoError(t, err)
	require.NoError(t, remote.Write(tag, img))
	gdb, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(&registry.ContainerRegistry{}))
	svc := registry.NewContainerRegistryService(&database.DB{DB: gdb}, nil, nil, nil, &http.Client{Transport: rewriteRegistryTransport{next: http.DefaultTransport, target: target}})
	resolver, err := NewResolver(svc, "")
	require.NoError(t, err)
	for _, raw := range []string{"nginx", "docker.io/library/nginx:latest"} {
		result, err := resolver.Resolve(t.Context(), dto.Request{Kind: "registry", Ref: raw})
		require.NoError(t, err, "Docker Hub needs no configured registry entry")
		var model map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(result.ComposeContent), &model))
		expected := raw
		if raw == "nginx" {
			expected = "nginx:latest"
		}
		require.Equal(t, expected, model["services"].(map[string]any)["nginx"].(map[string]any)["image"])
		require.Equal(t, "index.docker.io/library/nginx:latest", result.Source.CanonicalRef)
	}
}

func TestRegistryImportNestedIndexTimeout(t *testing.T) {
	inner := registryHTTP.New(registryHTTP.Logger(log.New(io.Discard, "", 0)))
	var slow atomic.Bool
	var slowManifest string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slow.Load() && r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/manifests/"+slowManifest) {
			timer := time.NewTimer(150 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-r.Context().Done():
				return
			case <-timer.C:
			}
		}
		inner.ServeHTTP(w, r)
	}))
	defer server.Close()
	img, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: runtime.GOOS, Architecture: runtime.GOARCH})
	require.NoError(t, err)
	digest, err := img.Digest()
	require.NoError(t, err)
	slowManifest = digest.String()
	nested := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: img, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: runtime.GOOS, Architecture: runtime.GOARCH}}})
	outer := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: nested})
	repo := strings.TrimPrefix(server.URL, "http://") + "/nested"
	tag, err := name.NewTag(repo+":1", name.Insecure)
	require.NoError(t, err)
	require.NoError(t, remote.WriteIndex(tag, outer))
	gdb, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(&registry.ContainerRegistry{}))
	require.NoError(t, gdb.Create(&registry.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true}).Error)
	svc := registry.NewContainerRegistryService(&database.DB{DB: gdb}, nil, nil, nil)
	resolver, err := NewResolver(svc, "")
	require.NoError(t, err)
	resolver.total, resolver.perRequest = time.Second, 30*time.Millisecond
	slow.Store(true)
	_, err = resolver.Resolve(t.Context(), dto.Request{Kind: "registry", Ref: tag.Name()})
	requireProblem(t, err, "import.timeout", 504)
}
