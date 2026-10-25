// Source: getarcaneapp/arcane/backend/internal/imageupdate/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed APNS mobile push; SQLite temp-file persistence fixture.

package imageupdate

import (
	"context"
	"encoding/json"

	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"

	"github.com/florianfinn/docklet-next/types/containerregistry"

	"github.com/moby/moby/api/pkg/authconfig"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	dockerregistry "github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/samber/mo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.getarcane.app/sys/crypto"
	"go.getarcane.app/updater/labels"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/kv"

	"github.com/florianfinn/docklet-next/backend/internal/registry"
)

type fakeRegistryDaemonClient struct {
	registryLoginFn       func(context.Context, client.RegistryLoginOptions) (client.RegistryLoginResult, error)
	distributionInspectFn func(context.Context, string, client.DistributionInspectOptions) (client.DistributionInspectResult, error)
}

func (f *fakeRegistryDaemonClient) RegistryLogin(ctx context.Context, options client.RegistryLoginOptions) (client.RegistryLoginResult, error) {
	if f.registryLoginFn == nil {
		return client.RegistryLoginResult{}, nil
	}
	return f.registryLoginFn(ctx, options)
}

func (f *fakeRegistryDaemonClient) DistributionInspect(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
	if f.distributionInspectFn == nil {
		return client.DistributionInspectResult{}, nil
	}
	return f.distributionInspectFn(ctx, imageRef, options)
}

// autoLabels opts a test container into tag-based update checks.
var autoLabels = map[string]string{labels.LabelUpdateStrategy: "auto"}

func newImageUpdateTestDockerClientInternal(t *testing.T, server *httptest.Server) *client.Client {
	t.Helper()
	cli, err := client.New(
		client.WithHost(server.URL),
		client.WithAPIVersion("1.41"),
		client.WithHTTPClient(server.Client()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

func setupImageUpdateRegistryTestDBInternal(t *testing.T) *database.DB {
	t.Helper()
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&registry.ContainerRegistry{},
		&kv.KVEntry{},
		&testProjectRow{},
		&ImageUpdateRecord{},
		&event.Event{},
	))
	crypto.InitEncryption(&crypto.Config{
		Environment:   "test",
		EncryptionKey: "test-encryption-key-for-testing-32bytes-min",
	})
	return &database.DB{DB: db}
}

func createImageUpdateTestPullRegistryInternal(t *testing.T, db *database.DB, localUrl, username, token string) {
	t.Helper()
	encryptedToken, err := crypto.Encrypt(token)
	require.NoError(t, err)
	require.NoError(t, db.Create(&registry.ContainerRegistry{
		URL:          localUrl,
		Username:     username,
		Token:        encryptedToken,
		Enabled:      true,
		RegistryType: registry.RegistryTypeGeneric,
	}).Error)
}

func decodeImageUpdateRegistryAuthInternal(t *testing.T, encoded string) dockerregistry.AuthConfig {
	t.Helper()
	config, err := authconfig.Decode(encoded)
	require.NoError(t, err)
	return *config
}

// TestParseImageReference tests the parseImageReference function with various image formats
// This is used for digest-based update checking
func TestImageUpdateService_ParseImageReference(t *testing.T) {
	alpineDigest := digest.FromString("alpine").String()
	serviceDigest := digest.FromString("registry-app-service").String()

	tests := []struct {
		name           string
		imageRef       string
		wantRegistry   string
		wantRepository string
		wantTag        string
	}{
		{
			name:           "Docker Hub official image with tag",
			imageRef:       "redis:latest",
			wantRegistry:   "docker.io",
			wantRepository: "library/redis",
			wantTag:        "latest",
		},
		{
			name:           "Docker Hub official image without tag",
			imageRef:       "nginx",
			wantRegistry:   "docker.io",
			wantRepository: "library/nginx",
			wantTag:        "latest",
		},
		{
			name:           "Docker Hub user image",
			imageRef:       "traefik/traefik:v2.10",
			wantRegistry:   "docker.io",
			wantRepository: "traefik/traefik",
			wantTag:        "v2.10",
		},
		{
			name:           "Custom registry with port",
			imageRef:       "localhost:5000/myapp:v1.0",
			wantRegistry:   "localhost:5000",
			wantRepository: "myapp",
			wantTag:        "v1.0",
		},
		{
			name:           "Custom registry with subdomain",
			imageRef:       "docker.getoutline.com/outlinewiki/outline:latest",
			wantRegistry:   "docker.getoutline.com",
			wantRepository: "outlinewiki/outline",
			wantTag:        "latest",
		},
		{
			name:           "GCR image",
			imageRef:       "gcr.io/google-containers/nginx:1.21",
			wantRegistry:   "gcr.io",
			wantRepository: "google-containers/nginx",
			wantTag:        "1.21",
		},
		{
			name:           "GHCR image",
			imageRef:       "ghcr.io/owner/repo:main",
			wantRegistry:   "ghcr.io",
			wantRepository: "owner/repo",
			wantTag:        "main",
		},
		{
			name:           "Multi-path repository",
			imageRef:       "registry.example.com/team/project/app:v2.0.0",
			wantRegistry:   "registry.example.com",
			wantRepository: "team/project/app",
			wantTag:        "v2.0.0",
		},
		{
			name:           "Image with digest",
			imageRef:       "alpine@" + alpineDigest,
			wantRegistry:   "docker.io",
			wantRepository: "library/alpine",
			wantTag:        "latest",
		},
		{
			name:           "Custom registry image with digest",
			imageRef:       "registry.io/app/service@" + serviceDigest,
			wantRegistry:   "registry.io",
			wantRepository: "app/service",
			wantTag:        "latest",
		},
	}

	svc := &ImageUpdateService{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := svc.parseImageReference(tt.imageRef)
			require.NotNil(t, parts, "parseImageReference returned nil")

			assert.Equal(t, tt.wantRegistry, parts.Registry, "registry mismatch")
			assert.Equal(t, tt.wantRepository, parts.Repository, "repository mismatch")
			assert.Equal(t, tt.wantTag, parts.Tag, "tag mismatch")
		})
	}
}

// TestParseImageReference_Fallback tests edge cases that might trigger fallback parsing
func TestImageUpdateService_ParseImageReference_Fallback(t *testing.T) {
	svc := &ImageUpdateService{}

	// Test malformed references that should still be parsed by fallback
	tests := []struct {
		name     string
		imageRef string
		wantNil  bool
	}{
		{
			name:     "Empty string",
			imageRef: "",
			wantNil:  false, // Fallback should handle it
		},
		{
			name:     "Valid reference",
			imageRef: "nginx:latest",
			wantNil:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := svc.parseImageReference(tt.imageRef)
			if tt.wantNil {
				assert.Nil(t, parts)
			} else {
				assert.NotNil(t, parts)
			}
		})
	}
}

// TestGetLocalImageDigestWithAll_ExtractsAllDigests tests that all digests are collected
func TestImageUpdateService_GetLocalImageDigestWithAll_Logic(t *testing.T) {
	// This is a unit test for the digest extraction logic
	// In a real scenario, you'd need to mock Docker client
	t.Run("Multiple digests in RepoDigests", func(t *testing.T) {
		// This test demonstrates the expected behavior
		// In practice, you'd use a mock Docker client
		firstDigest := digest.FromString("redis-primary").String()
		secondDigest := digest.FromString("redis-secondary").String()
		repoDigests := []string{
			"docker.io/library/redis@" + firstDigest,
			"redis@" + secondDigest,
		}

		var allDigests []string
		for _, repoDigest := range repoDigests {
			parts := splitRepoDigest(repoDigest)
			if parts != nil {
				allDigests = append(allDigests, parts.digest)
			}
		}

		assert.Len(t, allDigests, 2)
		assert.Contains(t, allDigests, firstDigest)
		assert.Contains(t, allDigests, secondDigest)
	})
}

// Helper function to test digest splitting
type repoDigestParts struct {
	repo   string
	digest string
}

func splitRepoDigest(repoDigest string) *repoDigestParts {
	parts := splitString(repoDigest, "@")
	if len(parts) == 2 {
		return &repoDigestParts{
			repo:   parts[0],
			digest: parts[1],
		}
	}
	return nil
}

func splitString(s, sep string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

// TestDockerReferenceCompatibility ensures our parser is compatible with Docker's reference package
func TestImageUpdateService_DockerReferenceCompatibility(t *testing.T) {
	tests := []struct {
		name     string
		imageRef string
	}{
		{"Docker Hub official", "nginx:latest"},
		{"Docker Hub user", "traefik/traefik:v2.0"},
		{"Custom registry", "gcr.io/project/app:v1"},
		{"With port", "localhost:5000/app:tag"},
		{"Multi-path", "registry.io/team/project/app:latest"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that official parser can handle it
			named, err := reference.ParseNormalizedNamed(tt.imageRef)
			require.NoError(t, err, "official parser failed")

			// Test our parser
			svc := &ImageUpdateService{}
			parts := svc.parseImageReference(tt.imageRef)
			require.NotNil(t, parts, "our parser returned nil")

			// Verify they produce the same results
			assert.Equal(t, reference.Domain(named), parts.Registry)
			assert.Equal(t, reference.Path(named), parts.Repository)
		})
	}
}

// TestStringToPtr tests the helper function for creating string pointers
func setupImageUpdateTestDB(t *testing.T) *database.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:image-update-test-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(testsqlite.Dialector(t, dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ImageUpdateRecord{}, &event.Event{}, &testProjectRow{}))
	return &database.DB{DB: db}
}

func newComposeBuildImageUpdateServiceInternal(t *testing.T) (*ImageUpdateService, *atomic.Int32) {
	t.Helper()

	db := setupImageUpdateTestDB(t)
	buildRefsJSON := `["test2:latest"]`
	require.NoError(t, db.Create(&testProjectRow{
		ID:                 "compose-build-project",
		Name:               "compose-build-project",
		BuildImageRefsJSON: &buildRefsJSON,
	}).Error)

	localDigest := digest.FromString("compose-build-local").String()
	remoteDigest := digest.FromString("compose-build-remote").String()
	dockerServer := newImageUpdateFallbackServer(t, "library/test2:latest", localDigest, remoteDigest)
	t.Cleanup(dockerServer.Close)

	var registryCalls atomic.Int32
	registryService := registry.NewContainerRegistryService(db, func(context.Context) (registry.RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(context.Context, string, client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				registryCalls.Add(1)
				return client.DistributionInspectResult{
					Descriptor: v1.Descriptor{Digest: digest.Digest(remoteDigest)},
				}, nil
			},
		}, nil
	}, nil, nil)

	dockerService := &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, dockerServer)}
	eventService := event.NewEventService(db, nil, nil)
	return NewImageUpdateService(db, nil, registryService, dockerService, eventService, nil, nil), &registryCalls
}

func TestImageUpdateService_CheckImageUpdate_ComposeBuildSkipsRegistryWithRepoDigests(t *testing.T) {
	svc, registryCalls := newComposeBuildImageUpdateServiceInternal(t)

	result, err := svc.CheckImageUpdate(t.Context(), "test2:latest")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, UpdateTypeLocal, result.UpdateType)
	assert.False(t, result.HasUpdate)
	assert.Empty(t, result.Error)
	assert.Zero(t, registryCalls.Load())

	var saved ImageUpdateRecord
	require.NoError(t, svc.db.First(&saved, "id = ?", "sha256:local-image-id").Error)
	assert.Equal(t, UpdateTypeLocal, saved.UpdateType)
	assert.Nil(t, saved.LastError)
}

func TestImageUpdateService_CheckMultipleImages_ComposeBuildSkipsRegistryWithRepoDigests(t *testing.T) {
	svc, registryCalls := newComposeBuildImageUpdateServiceInternal(t)
	credentials := []containerregistry.Credential{{
		URL:      "https://index.docker.io/v1/",
		Username: "unused",
		Token:    "unused",
		Enabled:  true,
	}}

	results, err := svc.CheckMultipleImages(t.Context(), []string{"test2:latest"}, credentials)
	require.NoError(t, err)
	result := results["test2:latest"]
	require.NotNil(t, result)
	assert.Equal(t, UpdateTypeLocal, result.UpdateType)
	assert.False(t, result.HasUpdate)
	assert.Empty(t, result.Error)
	assert.Zero(t, registryCalls.Load())
}

func TestImageUpdateService_CheckMultipleImages_ComposeBuildMissingLocallySkipsRegistry(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	buildRefsJSON := `["test2:latest"]`
	require.NoError(t, db.Create(&testProjectRow{
		ID:                 "compose-build-missing-project",
		Name:               "compose-build-missing-project",
		BuildImageRefsJSON: &buildRefsJSON,
	}).Error)

	dockerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{})) {
				return
			}
			return
		}

		http.Error(w, "No such image", http.StatusNotFound)
	}))
	t.Cleanup(dockerServer.Close)

	var registryCalls atomic.Int32
	registryService := registry.NewContainerRegistryService(db, func(context.Context) (registry.RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(context.Context, string, client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				registryCalls.Add(1)
				return client.DistributionInspectResult{}, nil
			},
		}, nil
	}, nil, nil)

	dockerService := &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, dockerServer)}
	eventService := event.NewEventService(db, nil, nil)
	svc := NewImageUpdateService(db, nil, registryService, dockerService, eventService, nil, nil)

	results, err := svc.CheckMultipleImages(t.Context(), []string{"test2:latest"}, nil)
	require.NoError(t, err)
	result := results["test2:latest"]
	require.NotNil(t, result)
	assert.Empty(t, result.Error)
	assert.False(t, result.HasUpdate)
	assert.Equal(t, UpdateTypeLocal, result.UpdateType)
	assert.Zero(t, registryCalls.Load())
}

func newArcaneLocalImageUpdateServiceInternal(t *testing.T, imageExists bool) (*ImageUpdateService, *atomic.Int32) {
	t.Helper()

	db := setupImageUpdateTestDB(t)
	localDigest := digest.FromString("arcane-local-build").String()
	dockerServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{})) {
				return
			}
			return
		}

		if imageExists && strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json") {
			w.Header().Set("Content-Type", "application/json")
			assert.NoError(t, json.NewEncoder(w).Encode(image.InspectResponse{
				ID:          "sha256:arcane-local-image-id",
				RepoTags:    []string{"arcane.local/demo-2ab41b29/worker:latest"},
				RepoDigests: []string{"arcane.local/demo-2ab41b29/worker@" + localDigest},
			}))
			return
		}
		http.Error(w, "No such image", http.StatusNotFound)
	}))
	t.Cleanup(dockerServer.Close)

	var registryCalls atomic.Int32
	registryService := registry.NewContainerRegistryService(db, func(context.Context) (registry.RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(context.Context, string, client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				registryCalls.Add(1)
				return client.DistributionInspectResult{
					Descriptor: v1.Descriptor{Digest: digest.FromString("arcane-local-remote")},
				}, nil
			},
		}, nil
	}, nil, nil)

	dockerService := &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, dockerServer)}
	eventService := event.NewEventService(db, nil, nil)
	return NewImageUpdateService(db, nil, registryService, dockerService, eventService, nil, nil), &registryCalls
}

func TestImageUpdateService_CheckImageUpdate_ArcaneLocalHostSkipsRegistry(t *testing.T) {
	svc, registryCalls := newArcaneLocalImageUpdateServiceInternal(t, true)
	staleError := "failed to get remote digest"
	require.NoError(t, svc.db.Create(&ImageUpdateRecord{
		ID:         "sha256:arcane-local-image-id",
		Repository: "arcane.local/demo-2ab41b29/worker",
		Tag:        "latest",
		LastError:  &staleError,
		CheckTime:  time.Now(),
	}).Error)

	result, err := svc.CheckImageUpdate(t.Context(), "arcane.local/demo-2ab41b29/worker:latest")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, UpdateTypeLocal, result.UpdateType)
	assert.False(t, result.HasUpdate)
	assert.Empty(t, result.Error)
	assert.Zero(t, registryCalls.Load())

	var saved ImageUpdateRecord
	require.NoError(t, svc.db.First(&saved, "id = ?", "sha256:arcane-local-image-id").Error)
	assert.Equal(t, UpdateTypeLocal, saved.UpdateType)
	assert.Nil(t, saved.LastError)
}

func TestImageUpdateService_CheckMultipleImages_ArcaneLocalMissingImageSkipsRegistry(t *testing.T) {
	svc, registryCalls := newArcaneLocalImageUpdateServiceInternal(t, false)

	results, err := svc.CheckMultipleImages(t.Context(), []string{"arcane.local/demo-2ab41b29/worker:latest"}, nil)
	require.NoError(t, err)
	result := results["arcane.local/demo-2ab41b29/worker:latest"]
	require.NotNil(t, result)
	assert.Empty(t, result.Error)
	assert.False(t, result.HasUpdate)
	assert.Equal(t, UpdateTypeLocal, result.UpdateType)
	assert.Zero(t, registryCalls.Load())
}

func TestImageUpdateService_CheckMultipleImages_OtherDottedRegistryStillChecked(t *testing.T) {
	svc, registryCalls := newArcaneLocalImageUpdateServiceInternal(t, false)

	results, err := svc.CheckMultipleImages(t.Context(), []string{"registry.local/team/app:latest"}, nil)
	require.NoError(t, err)
	result := results["registry.local/team/app:latest"]
	require.NotNil(t, result)
	assert.Empty(t, result.Error)
	assert.Equal(t, UpdateTypeNotPulled, result.UpdateType)
	assert.Positive(t, registryCalls.Load())
}

func TestImageUpdateService_InspectLocalImageSnapshot_NoRepoDigestsRemainsLocal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{})) {
				return
			}
			return
		}

		if strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json") {
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(image.InspectResponse{
				ID:       "sha256:local-only-image",
				RepoTags: []string{"local-only:latest"},
			})) {
				return
			}
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	svc := &ImageUpdateService{dockerService: &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)}}
	snapshot, err := svc.inspectLocalImageSnapshotInternal(t.Context(), "local-only:latest", map[string]struct{}{})
	require.NoError(t, err)
	assert.True(t, snapshot.IsLocalBuild)
	assert.Equal(t, "sha256:local-only-image", snapshot.PrimaryDigest)
}

// registryPingInternal answers the /v2/ version check every registry client sends before its first request.
func registryPingInternal(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/v2/" {
		return false
	}
	w.WriteHeader(http.StatusOK)
	return true
}

// writeManifestHeadInternal answers a manifest HEAD with the headers a client needs to trust the digest without a body.
func writeManifestHeadInternal(w http.ResponseWriter, localDigest string) {
	w.Header().Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
	w.Header().Set("Content-Length", "0")
	w.Header().Set("Docker-Content-Digest", localDigest)
	w.WriteHeader(http.StatusOK)
}

func newImageUpdateFallbackServer(t *testing.T, repositoryTag, localDigest, remoteDigest string) *httptest.Server {
	t.Helper()

	repository := repositoryTag
	tag := "latest"
	if tagIndex := strings.LastIndex(repositoryTag, ":"); tagIndex > strings.LastIndex(repositoryTag, "/") {
		repository = repositoryTag[:tagIndex]
		tag = repositoryTag[tagIndex+1:]
	}
	manifestPath := fmt.Sprintf("/v2/%s/manifests/%s", repository, tag)

	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{})) {
				return
			}
			return
		}

		switch {
		case strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
			imageRef := r.Host + "/" + repositoryTag
			repositoryRef := imageRef
			if tagIndex := strings.LastIndex(imageRef, ":"); tagIndex > strings.LastIndex(imageRef, "/") {
				repositoryRef = imageRef[:tagIndex]
			}

			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(image.InspectResponse{
				ID:          "sha256:local-image-id",
				RepoTags:    []string{imageRef},
				RepoDigests: []string{repositoryRef + "@" + localDigest},
			})) {
				return
			}
			return
		case r.URL.Path == manifestPath:
			writeManifestHeadInternal(w, remoteDigest)
			return
		case registryPingInternal(w, r):
			return
		default:
			http.NotFound(w, r)
		}
	}))
}

func newImageUpdateRegistryOnlyServer(t *testing.T, repositoryTag, remoteDigest string) *httptest.Server {
	t.Helper()

	repository := repositoryTag
	tag := "latest"
	if tagIndex := strings.LastIndex(repositoryTag, ":"); tagIndex > strings.LastIndex(repositoryTag, "/") {
		repository = repositoryTag[:tagIndex]
		tag = repositoryTag[tagIndex+1:]
	}
	manifestPath := fmt.Sprintf("/v2/%s/manifests/%s", repository, tag)

	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{})) {
				return
			}
			return
		}

		switch {
		case strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
			http.Error(w, "not found", http.StatusNotFound)
			return
		case r.URL.Path == manifestPath:
			writeManifestHeadInternal(w, remoteDigest)
			return
		case registryPingInternal(w, r):
			return
		default:
			http.NotFound(w, r)
		}
	}))
}

func newImageRefResolutionServer(t *testing.T, containers []container.Summary) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
			http.Error(w, "not found", http.StatusNotFound)
			return
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(containers)) {
				return
			}
			return
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestImageUpdateService_GetImageRefByIDInternal_UsesContainerFallback(t *testing.T) {
	t.Parallel()

	const imageID = "sha256:test-image-id"

	tests := []struct {
		name       string
		containers []container.Summary
		wantRef    string
		wantErr    string
	}{
		{
			name: "uses repo tag from matching container when inspect fails",
			containers: []container.Summary{
				{ImageID: imageID, Image: "frooodle/s-pdf:latest"},
			},
			wantRef: "frooodle/s-pdf:latest",
		},
		{
			name: "ignores named digest references from matching container",
			containers: []container.Summary{
				{ImageID: imageID, Image: "frooodle/s-pdf@sha256:abc123"},
			},
			wantErr: "no local image or running container found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newImageRefResolutionServer(t, tt.containers)
			defer server.Close()

			svc := &ImageUpdateService{
				dockerService: &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)},
			}

			imageRef, err := svc.getImageRefByIDInternal(t.Context(), imageID)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.wantErr)
				assert.Empty(t, imageRef)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantRef, imageRef)
		})
	}
}

func TestImageUpdateService_CheckImageUpdate_SkipsDigestPinnedReferenceInternal(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	pinnedDigest := digest.FromString("pinned-newt").String()
	imageRef := "ghcr.io/fosrl/newt@" + pinnedDigest
	eventService := event.NewEventService(db, nil, nil)
	svc := NewImageUpdateService(db, nil, nil, nil, eventService, nil, nil)

	result, err := svc.CheckImageUpdate(t.Context(), imageRef)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.HasUpdate)
	assert.Equal(t, "digest", result.UpdateType)
	assert.Equal(t, pinnedDigest, result.CurrentDigest)
	assert.Equal(t, pinnedDigest, result.LatestDigest)
	assert.Empty(t, result.Error)
}

func TestImageUpdateService_CheckMultipleImages_SkipsDigestPinnedReferenceInternal(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	pinnedDigest := digest.FromString("batch-pinned-newt").String()
	imageRef := "ghcr.io/fosrl/newt@" + pinnedDigest
	svc := NewImageUpdateService(db, nil, nil, nil, nil, nil, nil)

	regRepos, initialResults, grouped := svc.parseAndGroupImagesInternal([]string{imageRef})
	require.Empty(t, regRepos)
	require.Empty(t, grouped)
	require.Contains(t, initialResults, imageRef)
	require.NotNil(t, initialResults[imageRef])
	assert.Empty(t, initialResults[imageRef].Error)

	results, err := svc.CheckMultipleImages(t.Context(), []string{imageRef}, nil)
	require.NoError(t, err)
	require.Contains(t, results, imageRef)
	require.NotNil(t, results[imageRef])
	assert.False(t, results[imageRef].HasUpdate)
	assert.Equal(t, "digest", results[imageRef].UpdateType)
	assert.Equal(t, pinnedDigest, results[imageRef].CurrentDigest)
	assert.Equal(t, pinnedDigest, results[imageRef].LatestDigest)
	assert.Empty(t, results[imageRef].Error)
}

func TestImageUpdateService_CheckMultipleImages_SkippedDigestPinnedReferenceClearsStaleErrorInternal(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	pinnedDigest := digest.FromString("stale-pinned-newt").String()
	imageRef := "ghcr.io/fosrl/newt@" + pinnedDigest
	repository := "ghcr.io/fosrl/newt"
	recordID := fmt.Sprintf("ref::%s@latest", strings.ToLower(strings.TrimSpace(repository)))
	require.NoError(t, db.Create(&ImageUpdateRecord{
		ID:             recordID,
		Repository:     repository,
		Tag:            "latest",
		HasUpdate:      false,
		UpdateType:     "digest",
		CurrentVersion: "latest",
		LastError:      new("failed to get remote digest"),
		CheckTime:      time.Now().Add(-time.Hour),
	}).Error)
	svc := NewImageUpdateService(db, nil, nil, nil, nil, nil, nil)

	results, err := svc.CheckMultipleImages(t.Context(), []string{imageRef}, nil)
	require.NoError(t, err)
	require.Contains(t, results, imageRef)
	assert.Empty(t, results[imageRef].Error)

	var saved ImageUpdateRecord
	require.NoError(t, db.WithContext(t.Context()).Where("id = ?", recordID).First(&saved).Error)
	assert.False(t, saved.HasUpdate)
	assert.Nil(t, saved.LastError)
	assert.Equal(t, pinnedDigest, mo.PointerToOption(saved.CurrentDigest).OrEmpty())
	assert.Equal(t, pinnedDigest, mo.PointerToOption(saved.LatestDigest).OrEmpty())
}

func TestImageUpdateService_CheckMultipleImages_DigestPinnedTagPreservedWhenLocalImageHasNoRepoTags(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	pinnedDigest := digest.FromString("valkey-pinned").String()

	server := newImageUpdateNoRepoTagsServer(t, "sha256:pinned-local-id", pinnedDigest)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	repository := serverURL.Host + "/valkey/valkey"
	imageRef := repository + ":9@" + pinnedDigest

	dockerService := &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)}
	svc := NewImageUpdateService(db, nil, nil, dockerService, nil, nil, nil)

	results, err := svc.CheckMultipleImages(t.Context(), []string{imageRef}, nil)
	require.NoError(t, err)
	require.Contains(t, results, imageRef)
	assert.False(t, results[imageRef].HasUpdate)
	assert.Empty(t, results[imageRef].Error)

	var saved ImageUpdateRecord
	require.NoError(t, db.WithContext(t.Context()).Where("id = ?", "sha256:pinned-local-id").First(&saved).Error)
	assert.Equal(t, "9", saved.Tag, "tag should come from the original tag@digest reference, not fall back to the RepoDigests-only placeholder")
}

func newImageUpdateNoRepoTagsServer(t *testing.T, imageID, localDigest string) *httptest.Server {
	t.Helper()

	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{})) {
				return
			}
			return
		}

		if strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json") {
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(image.InspectResponse{
				ID:          imageID,
				RepoTags:    []string{},
				RepoDigests: []string{r.Host + "/valkey/valkey@" + localDigest},
			})) {
				return
			}
			return
		}
		http.NotFound(w, r)
	}))
}
