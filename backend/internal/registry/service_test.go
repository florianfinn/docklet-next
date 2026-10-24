// Source: getarcaneapp/arcane/backend/internal/registry/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; SQLite temp-file persistence fixture.

package registry

import (
	"context"

	"errors"

	"net/http"
	"net/http/httptest"
	"net/url"

	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/containerregistry"

	"github.com/moby/moby/api/pkg/authconfig"
	dockerregistry "github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
)

type fakeRegistryDaemonClient struct {
	registryLoginFn       func(ctx context.Context, options client.RegistryLoginOptions) (client.RegistryLoginResult, error)
	distributionInspectFn func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error)
}

func setupContainerRegistryTestDBInternal(t *testing.T) *database.DB {
	t.Helper()

	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ContainerRegistry{}, &kv.KVEntry{}))
	require.NoError(t, db.Exec("CREATE TABLE IF NOT EXISTS projects (id text PRIMARY KEY, name text, path text)").Error)

	crypto.InitEncryption(&crypto.Config{
		Environment:   "test",
		EncryptionKey: "test-encryption-key-for-testing-32bytes-min",
	})

	return &database.DB{DB: db}
}

func createTestPullRegistryInternal(t *testing.T, db *database.DB, localUrl, username, token string) {
	t.Helper()

	encryptedToken, err := crypto.Encrypt(token)
	require.NoError(t, err)

	record := &ContainerRegistry{
		URL:          localUrl,
		Username:     username,
		Token:        encryptedToken,
		Enabled:      true,
		RegistryType: RegistryTypeGeneric,
	}
	require.NoError(t, db.WithContext(t.Context()).Create(record).Error)
}

func decodeRegistryAuthInternal(t *testing.T, encoded string) dockerregistry.AuthConfig {
	t.Helper()

	cfg, err := authconfig.Decode(encoded)
	require.NoError(t, err)
	return *cfg
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
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

const dockerHubChallengeInternal = `Bearer realm="https://auth.docker.io/token",service="registry.docker.io"`

// registryPingInternal answers the /v2/ version check every registry client sends first: a bearer challenge when set, else OK.
func registryPingInternal(w http.ResponseWriter, r *http.Request, challenge string) bool {
	if r.URL.Path != "/v2/" {
		return false
	}
	if challenge != "" {
		w.Header().Set("WWW-Authenticate", challenge)
		w.WriteHeader(http.StatusUnauthorized)
		return true
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

func newDockerHubRateLimitTestClient(t *testing.T, handler http.HandlerFunc) *http.Client {
	t.Helper()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if registryPingInternal(w, r, dockerHubChallengeInternal) {
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	targetURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	httpClient := server.Client()
	baseTransport := httpClient.Transport
	httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "registry-1.docker.io" || req.URL.Host == "auth.docker.io" {
			rewritten := req.Clone(req.Context())
			rewritten.URL.Scheme = targetURL.Scheme
			rewritten.URL.Host = targetURL.Host
			return baseTransport.RoundTrip(rewritten)
		}

		return baseTransport.RoundTrip(req)
	})

	return httpClient
}

func TestNewContainerRegistryService_InitializesDistributionHTTPClient(t *testing.T) {
	svc := NewContainerRegistryService(nil, nil, nil, nil)
	require.NotNil(t, svc.distributionHTTPClient)
}

func TestContainerRegistryService_GetAllRegistryAuthConfigs_NormalizesHosts(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "https://index.docker.io/v1/", "docker-user", "docker-token")
	createTestPullRegistryInternal(t, db, "https://GHCR.IO/", "gh-user", "gh-token")

	svc := NewContainerRegistryService(db, nil, nil, nil)
	authConfigs, err := svc.GetAllRegistryAuthConfigs(t.Context())
	require.NoError(t, err)
	require.NotNil(t, authConfigs)

	dockerCfg, ok := authConfigs["docker.io"]
	require.True(t, ok)
	assert.Equal(t, "docker-user", dockerCfg.Username)
	assert.Equal(t, "docker-token", dockerCfg.Password)
	assert.Equal(t, "https://index.docker.io/v1/", dockerCfg.ServerAddress)

	assert.Equal(t, dockerCfg, authConfigs["registry-1.docker.io"])
	assert.Equal(t, dockerCfg, authConfigs["index.docker.io"])

	ghcrCfg, ok := authConfigs["ghcr.io"]
	require.True(t, ok)
	assert.Equal(t, "gh-user", ghcrCfg.Username)
	assert.Equal(t, "gh-token", ghcrCfg.Password)
	assert.Equal(t, "ghcr.io", ghcrCfg.ServerAddress)
}

func TestContainerRegistryService_GetAllRegistryAuthConfigs_SkipsInvalidEntries(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "https://docker.io", "  ", "docker-token")
	createTestPullRegistryInternal(t, db, "https://ghcr.io", "gh-user", "   ")
	createTestPullRegistryInternal(t, db, "https://registry.example.com", "example-user", "example-token")

	svc := NewContainerRegistryService(db, nil, nil, nil)
	authConfigs, err := svc.GetAllRegistryAuthConfigs(t.Context())
	require.NoError(t, err)
	require.NotNil(t, authConfigs)

	assert.NotContains(t, authConfigs, "docker.io")
	assert.NotContains(t, authConfigs, "ghcr.io")

	exampleCfg, ok := authConfigs["registry.example.com"]
	require.True(t, ok)
	assert.Equal(t, "example-user", exampleCfg.Username)
	assert.Equal(t, "example-token", exampleCfg.Password)
	assert.Equal(t, "registry.example.com", exampleCfg.ServerAddress)
}

func TestContainerRegistryService_GetRegistryAuthForHost_UsesDatabaseCredentials(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "https://index.docker.io/v1/", "docker-user", "docker-token")

	svc := NewContainerRegistryService(db, nil, nil, nil)
	auth, err := svc.GetRegistryAuthForHost(t.Context(), "registry-1.docker.io")
	require.NoError(t, err)
	require.NotEmpty(t, auth)

	cfg := decodeRegistryAuthInternal(t, auth)
	assert.Equal(t, "docker-user", cfg.Username)
	assert.Equal(t, "docker-token", cfg.Password)
	assert.Equal(t, "https://index.docker.io/v1/", cfg.ServerAddress)
}

func TestContainerRegistryService_GetRegistryAuthForImage_UsesHostLookup(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "https://ghcr.io", "gh-user", "gh-token")

	svc := NewContainerRegistryService(db, nil, nil, nil)
	auth, err := svc.GetRegistryAuthForImage(t.Context(), "ghcr.io/getarcaneapp/arcane:latest")
	require.NoError(t, err)
	require.NotEmpty(t, auth)

	cfg := decodeRegistryAuthInternal(t, auth)
	assert.Equal(t, "gh-user", cfg.Username)
	assert.Equal(t, "gh-token", cfg.Password)
	assert.Equal(t, "ghcr.io", cfg.ServerAddress)
}

func TestContainerRegistryService_NilService_RegistryAuthMethodsReturnEmpty(t *testing.T) {
	var svc *ContainerRegistryService
	ctx := t.Context()

	authConfigs, err := svc.GetAllRegistryAuthConfigs(ctx)
	require.NoError(t, err)
	require.Nil(t, authConfigs)

	authForHost, err := svc.GetRegistryAuthForHost(ctx, "registry-1.docker.io")
	require.NoError(t, err)
	require.Empty(t, authForHost)

	authForImage, err := svc.GetRegistryAuthForImage(ctx, "ghcr.io/getarcaneapp/arcane:latest")
	require.NoError(t, err)
	require.Empty(t, authForImage)
}

func TestContainerRegistryService_GetRegistryPullUsage_AnonymousDockerHubLimit(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	require.NoError(t, db.WithContext(t.Context()).Create(&ContainerRegistry{
		URL:          "docker.io",
		Enabled:      true,
		RegistryType: RegistryTypeGeneric,
	}).Error)

	var tokenCalls int

	svc := NewContainerRegistryService(db, nil, kv.NewKVService(db), nil)
	svc.distributionHTTPClient = newDockerHubRateLimitTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			tokenCalls++
			assert.Empty(t, r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"token":"anonymous-token"}`))
		case r.Method == http.MethodHead && r.URL.Path == "/v2/ratelimitpreview/test/manifests/latest":
			if r.Header.Get("Authorization") != "Bearer anonymous-token" {
				w.Header().Set("WWW-Authenticate", dockerHubChallengeInternal)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("RateLimit-Limit", "100;w=21600")
			w.Header().Set("RateLimit-Remaining", "90;w=21600")
			w.WriteHeader(http.StatusOK)
		default:
			if !assert.Failf(t, "unexpected failure", "unexpected request %s %s", r.Method, r.URL.Path) {
				return
			}
		}
	})

	result, err := svc.GetRegistryPullUsage(t.Context())
	require.NoError(t, err)

	require.Len(t, result.Registries, 1)
	registryRecord := result.Registries[0]
	assert.Equal(t, "dockerhub", registryRecord.Provider)
	assert.Equal(t, "anonymous", registryRecord.AuthMethod)
	assert.Empty(t, registryRecord.Error)
	require.NotNil(t, registryRecord.Limit)
	require.NotNil(t, registryRecord.Remaining)
	require.NotNil(t, registryRecord.Used)
	assert.Equal(t, 100, *registryRecord.Limit)
	assert.Equal(t, 90, *registryRecord.Remaining)
	assert.Equal(t, 10, *registryRecord.Used)
	assert.Equal(t, 1, tokenCalls)

	cachedResult, err := svc.GetRegistryPullUsage(t.Context())
	require.NoError(t, err)
	require.Len(t, cachedResult.Registries, 1)
	require.NotNil(t, cachedResult.Registries[0].Remaining)
	assert.Equal(t, 90, *cachedResult.Registries[0].Remaining)
	assert.Equal(t, 1, tokenCalls)
}

func TestContainerRegistryService_GetRegistryPullUsage_UsesDockerHubCredential(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "https://index.docker.io/v1/", "docker-user", "docker-token")
	createTestPullRegistryInternal(t, db, "https://ghcr.io", "gh-user", "gh-token")

	var tokenAuth string
	svc := NewContainerRegistryService(db, nil, kv.NewKVService(db), nil)
	svc.distributionHTTPClient = newDockerHubRateLimitTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			tokenAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"token":"credential-token"}`))
		case r.Method == http.MethodHead && r.URL.Path == "/v2/ratelimitpreview/test/manifests/latest":
			if r.Header.Get("Authorization") != "Bearer credential-token" {
				w.Header().Set("WWW-Authenticate", dockerHubChallengeInternal)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("RateLimit-Limit", "200;w=21600")
			w.Header().Set("RateLimit-Remaining", "199;w=21600")
			w.WriteHeader(http.StatusOK)
		default:
			if !assert.Failf(t, "unexpected failure", "unexpected request %s %s", r.Method, r.URL.Path) {
				return
			}
		}
	})

	result, err := svc.GetRegistryPullUsage(t.Context())
	require.NoError(t, err)

	require.Len(t, result.Registries, 2)
	registryRecord := result.Registries[0]
	assert.Equal(t, "credential", registryRecord.AuthMethod)
	assert.Equal(t, "docker-user", registryRecord.AuthUsername)
	assert.Empty(t, registryRecord.Error)
	require.NotNil(t, registryRecord.Limit)
	require.NotNil(t, registryRecord.Remaining)
	assert.Equal(t, 200, *registryRecord.Limit)
	assert.Equal(t, 199, *registryRecord.Remaining)
	assert.Contains(t, tokenAuth, "Basic ")
}

func TestContainerRegistryService_GetRegistryPullUsage_CredentialErrorIsNonFatal(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "https://docker.io", "docker-user", "docker-token")

	svc := NewContainerRegistryService(db, nil, kv.NewKVService(db), nil)
	svc.distributionHTTPClient = newDockerHubRateLimitTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			w.WriteHeader(http.StatusUnauthorized)
		case r.Method == http.MethodHead && r.URL.Path == "/v2/ratelimitpreview/test/manifests/latest":
			w.Header().Set("WWW-Authenticate", dockerHubChallengeInternal)
			w.WriteHeader(http.StatusUnauthorized)
		default:
			if !assert.Failf(t, "unexpected failure", "unexpected request %s %s", r.Method, r.URL.Path) {
				return
			}
		}
	})

	result, err := svc.GetRegistryPullUsage(t.Context())
	require.NoError(t, err)

	require.Len(t, result.Registries, 1)
	registryRecord := result.Registries[0]
	assert.Equal(t, "credential", registryRecord.AuthMethod)
	assert.Equal(t, "docker-user", registryRecord.AuthUsername)
	assert.Equal(t, "registry.unavailable", registryRecord.Code)
	assert.NotContains(t, registryRecord.Error, "401 Unauthorized")
}

func TestContainerRegistryService_RecordImagePull_IncrementsObservedRegistryCount(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	require.NoError(t, db.WithContext(t.Context()).Create(&ContainerRegistry{
		URL:          "ghcr.io",
		Enabled:      true,
		RegistryType: RegistryTypeGeneric,
	}).Error)

	svc := NewContainerRegistryService(db, nil, kv.NewKVService(db), nil)
	require.NoError(t, svc.RecordImagePull(t.Context(), "ghcr.io/example/app:latest"))
	require.NoError(t, svc.RecordImagePull(t.Context(), "ghcr.io/example/worker:latest"))

	result, err := svc.GetRegistryPullUsage(t.Context())
	require.NoError(t, err)
	require.Len(t, result.Registries, 1)
	assert.Equal(t, "ghcr.io", result.Registries[0].Registry)
	assert.Equal(t, int64(2), result.Registries[0].ObservedPulls)
	assert.Nil(t, result.Registries[0].Limit)
}

func TestContainerRegistryService_CreateRegistry_RejectsUnsupportedRegistryType(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	svc := NewContainerRegistryService(db, nil, nil, nil)

	_, err := svc.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{
		URL:          "registry.example.com",
		RegistryType: "ECR-ish",
	})
	require.Error(t, err)

	require.ErrorIs(t, err, common.ErrValidation)
	fieldErr, ok := errors.AsType[*base.FieldError](err)
	require.True(t, ok)
	assert.Equal(t, "registryType", fieldErr.Field)
}

func TestContainerRegistryService_CreateRegistry_RejectsEmptyUsernameForGeneric(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	svc := NewContainerRegistryService(db, nil, nil, nil)

	_, err := svc.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{
		URL:      "https://registry.example.com",
		Username: "",
		Token:    "my-token",
	})
	require.Error(t, err)

	require.ErrorIs(t, err, common.ErrValidation)
	fieldErr, ok := errors.AsType[*base.FieldError](err)
	require.True(t, ok)
	assert.Equal(t, "username", fieldErr.Field)
}

func TestContainerRegistryService_CreateRegistry_RejectsEmptyTokenForGeneric(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	svc := NewContainerRegistryService(db, nil, nil, nil)

	_, err := svc.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{
		URL:      "https://registry.example.com",
		Username: "my-user",
		Token:    "",
	})
	require.Error(t, err)

	require.ErrorIs(t, err, common.ErrValidation)
	fieldErr, ok := errors.AsType[*base.FieldError](err)
	require.True(t, ok)
	assert.Equal(t, "token", fieldErr.Field)
}

func TestContainerRegistryService_CreateRegistry_AcceptsValidGenericCredentials(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	svc := NewContainerRegistryService(db, nil, nil, nil)

	reg, err := svc.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{
		URL:      "https://registry.example.com",
		Username: "my-user",
		Token:    "my-token",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-user", reg.Username)
	assert.NotEmpty(t, reg.Token)
}

func TestContainerRegistryService_UpdateRegistry_RejectsBlankingUsername(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	svc := NewContainerRegistryService(db, nil, nil, nil)

	reg, err := svc.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{
		URL:      "https://registry.example.com",
		Username: "my-user",
		Token:    "my-token",
	})
	require.NoError(t, err)

	_, err = svc.UpdateRegistry(t.Context(), reg.ID, containerregistry.UpdateContainerRegistryRequest{
		Username: new(""),
	})
	require.Error(t, err)

	require.ErrorIs(t, err, common.ErrValidation)
	fieldErr, ok := errors.AsType[*base.FieldError](err)
	require.True(t, ok)
	assert.Equal(t, "username", fieldErr.Field)
}

func TestContainerRegistryService_UpdateRegistry_KeepsExistingTokenWhenNotProvided(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	svc := NewContainerRegistryService(db, nil, nil, nil)

	reg, err := svc.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{
		URL:      "https://registry.example.com",
		Username: "my-user",
		Token:    "my-token",
	})
	require.NoError(t, err)
	originalToken := reg.Token

	updated, err := svc.UpdateRegistry(t.Context(), reg.ID, containerregistry.UpdateContainerRegistryRequest{
		Username: new("updated-user"),
	})
	require.NoError(t, err)
	assert.Equal(t, "updated-user", updated.Username)
	assert.Equal(t, originalToken, updated.Token)
}

func TestContainerRegistryService_UpdateRegistry_RejectsTargetChangeWhenStoredTokenWouldBeReused(t *testing.T) {
	tests := []struct {
		name  string
		token *string
	}{
		{name: "omitted token"},
		{name: "empty token", token: new("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupContainerRegistryTestDBInternal(t)
			svc := NewContainerRegistryService(db, nil, nil, nil)

			registryRecord, err := svc.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{
				URL:      "https://registry.example.com",
				Username: "my-user",
				Token:    "my-token",
			})
			require.NoError(t, err)
			originalToken := registryRecord.Token

			_, err = svc.UpdateRegistry(t.Context(), registryRecord.ID, containerregistry.UpdateContainerRegistryRequest{
				URL:   new("https://attacker.example.com"),
				Token: tt.token,
			})
			require.Error(t, err)

			require.ErrorIs(t, err, common.ErrValidation)
			fieldErr, ok := errors.AsType[*base.FieldError](err)
			require.True(t, ok)
			assert.Equal(t, "token", fieldErr.Field)

			stored, loadErr := svc.GetRegistryByID(t.Context(), registryRecord.ID)
			require.NoError(t, loadErr)
			assert.Equal(t, "https://registry.example.com", stored.URL)
			assert.Equal(t, originalToken, stored.Token)
		})
	}
}

func TestContainerRegistryService_UpdateRegistry_AllowsPathChangeOnSameHostWithoutToken(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	svc := NewContainerRegistryService(db, nil, nil, nil)

	registryRecord, err := svc.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{
		URL:      "https://registry.example.com/one",
		Username: "my-user",
		Token:    "my-token",
	})
	require.NoError(t, err)
	originalToken := registryRecord.Token

	updated, err := svc.UpdateRegistry(t.Context(), registryRecord.ID, containerregistry.UpdateContainerRegistryRequest{
		URL: new("REGISTRY.EXAMPLE.COM/two"),
	})
	require.NoError(t, err)
	assert.Equal(t, "REGISTRY.EXAMPLE.COM/two", updated.URL)
	assert.Equal(t, originalToken, updated.Token)
}

func TestContainerRegistryService_UpdateRegistry_AllowsTargetChangeWhenTokenIsResupplied(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	svc := NewContainerRegistryService(db, nil, nil, nil)

	registryRecord, err := svc.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{
		URL:      "https://registry.example.com",
		Username: "my-user",
		Token:    "my-token",
	})
	require.NoError(t, err)

	updated, err := svc.UpdateRegistry(t.Context(), registryRecord.ID, containerregistry.UpdateContainerRegistryRequest{
		URL:   new("https://registry.example.net"),
		Token: new("new-token"),
	})
	require.NoError(t, err)
	assert.Equal(t, "https://registry.example.net", updated.URL)

	decryptedToken, decryptErr := crypto.Decrypt(updated.Token)
	require.NoError(t, decryptErr)
	assert.Equal(t, "new-token", decryptedToken)
}

func TestContainerRegistryService_UpdateRegistryRejectsECRTargetChangeWithStoredCredentialsInternal(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	svc := NewContainerRegistryService(db, nil, nil, nil)

	registryRecord, err := svc.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{
		URL:                "123456789012.dkr.ecr.us-east-1.amazonaws.com",
		RegistryType:       RegistryTypeECR,
		AWSAccessKeyID:     "old-access-key",
		AWSSecretAccessKey: "old-secret-key",
		AWSRegion:          "us-east-1",
	})
	require.NoError(t, err)
	originalSecret := registryRecord.AWSSecretAccessKey

	_, err = svc.UpdateRegistry(t.Context(), registryRecord.ID, containerregistry.UpdateContainerRegistryRequest{
		URL: new("999999999999.dkr.ecr.us-east-1.amazonaws.com"),
	})
	require.Error(t, err)

	var apiErr *common.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, common.APIErrorCodeValidationError, apiErr.Code)
	require.ElementsMatch(t, []string{"awsAccessKeyId", "awsSecretAccessKey"}, apiErr.Details.(map[string]any)["fields"])

	stored, loadErr := svc.GetRegistryByID(t.Context(), registryRecord.ID)
	require.NoError(t, loadErr)
	require.Equal(t, "123456789012.dkr.ecr.us-east-1.amazonaws.com", stored.URL)
	require.Equal(t, originalSecret, stored.AWSSecretAccessKey)

	updated, err := svc.UpdateRegistry(t.Context(), registryRecord.ID, containerregistry.UpdateContainerRegistryRequest{
		URL:                new("999999999999.dkr.ecr.us-east-1.amazonaws.com"),
		AWSAccessKeyID:     new("new-access-key"),
		AWSSecretAccessKey: new("new-secret-key"),
	})
	require.NoError(t, err)
	require.Equal(t, "999999999999.dkr.ecr.us-east-1.amazonaws.com", updated.URL)

	decryptedSecret, decryptErr := crypto.Decrypt(updated.AWSSecretAccessKey)
	require.NoError(t, decryptErr)
	require.Equal(t, "new-secret-key", decryptedSecret)
}

func TestContainerRegistryService_UpdateRegistry_RejectsChangingRegistryType(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	svc := NewContainerRegistryService(db, nil, nil, nil)

	reg, err := svc.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{
		URL:      "https://registry.example.com",
		Username: "my-user",
		Token:    "my-token",
	})
	require.NoError(t, err)

	_, err = svc.UpdateRegistry(t.Context(), reg.ID, containerregistry.UpdateContainerRegistryRequest{
		RegistryType: new("ecr"),
	})
	require.Error(t, err)

	require.ErrorIs(t, err, common.ErrValidation)
	fieldErr, ok := errors.AsType[*base.FieldError](err)
	require.True(t, ok)
	assert.Equal(t, "registryType", fieldErr.Field)
}

func TestContainerRegistryService_UpdateRegistry_AllowsSameRegistryType(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	svc := NewContainerRegistryService(db, nil, nil, nil)

	reg, err := svc.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{
		URL:      "https://registry.example.com",
		Username: "my-user",
		Token:    "my-token",
	})
	require.NoError(t, err)

	updated, err := svc.UpdateRegistry(t.Context(), reg.ID, containerregistry.UpdateContainerRegistryRequest{
		RegistryType: new("generic"),
		Username:     new("updated-user"),
	})
	require.NoError(t, err)
	assert.Equal(t, "updated-user", updated.Username)
	assert.Equal(t, "generic", updated.RegistryType)
}

func TestContainerRegistryService_SyncRegistries_ClearsGenericTokenWhenManagerSendsEmptyValue(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "https://registry.example.com", "registry-user", "old-token")

	var existing ContainerRegistry
	require.NoError(t, db.WithContext(t.Context()).First(&existing).Error)

	svc := NewContainerRegistryService(db, nil, nil, nil)
	err := svc.SyncRegistries(t.Context(), []containerregistry.Sync{
		{
			ID:           existing.ID,
			URL:          existing.URL,
			Username:     existing.Username,
			Token:        "",
			Enabled:      true,
			RegistryType: RegistryTypeGeneric,
			CreatedAt:    existing.CreatedAt,
			UpdatedAt:    existing.UpdatedAt,
		},
	})
	require.NoError(t, err)

	var updated ContainerRegistry
	require.NoError(t, db.WithContext(t.Context()).First(&updated, "id = ?", existing.ID).Error)

	decryptedToken, err := crypto.Decrypt(updated.Token)
	require.NoError(t, err)
	assert.Empty(t, decryptedToken)
}

func TestContainerRegistryService_TestRegistry_UsesDockerDaemon(t *testing.T) {
	svc := NewContainerRegistryService(nil, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			registryLoginFn: func(ctx context.Context, options client.RegistryLoginOptions) (client.RegistryLoginResult, error) {
				assert.Equal(t, "user", options.Username)
				assert.Equal(t, "token", options.Password)
				assert.Equal(t, "registry.example.com:5443", options.ServerAddress)
				return client.RegistryLoginResult{}, nil
			},
		}, nil
	}, nil, nil)

	err := svc.TestRegistry(t.Context(), "https://registry.example.com:5443", "user", "token")
	require.NoError(t, err)
}

func TestContainerRegistryService_TestRegistry_PropagatesDaemonError(t *testing.T) {
	expectedErr := errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority")
	svc := NewContainerRegistryService(nil, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			registryLoginFn: func(ctx context.Context, options client.RegistryLoginOptions) (client.RegistryLoginResult, error) {
				return client.RegistryLoginResult{}, expectedErr
			},
		}, nil
	}, nil, nil)

	err := svc.TestRegistry(t.Context(), "registry.example.com", "user", "token")
	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
}

func TestContainerRegistryService_TestRegistry_SkipsLoginForEmptyCredentials(t *testing.T) {
	svc := NewContainerRegistryService(nil, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			registryLoginFn: func(ctx context.Context, options client.RegistryLoginOptions) (client.RegistryLoginResult, error) {
				require.FailNow(t, "RegistryLogin should not be called with empty credentials")
				return client.RegistryLoginResult{}, nil
			},
		}, nil
	}, nil, nil)

	err := svc.TestRegistry(t.Context(), "registry.example.com", "", "")
	require.NoError(t, err)

	err = svc.TestRegistry(t.Context(), "registry.example.com", "  ", "  ")
	require.NoError(t, err)
}

func TestContainerRegistryService_InspectImageDigest_AnonymousSuccess(t *testing.T) {
	wantDigest := digest.FromString("anonymous-success").String()

	svc := NewContainerRegistryService(nil, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				assert.Equal(t, "registry.example.com:5443/team/app:1.2.3", imageRef)
				assert.Empty(t, options.EncodedRegistryAuth)
				return client.DistributionInspectResult{
					Descriptor: ocispec.Descriptor{
						Digest: digest.Digest(wantDigest),
					},
				}, nil
			},
		}, nil
	}, nil, nil)

	result, err := svc.InspectImageDigest(t.Context(), "registry.example.com:5443/team/app:1.2.3", nil)
	require.NoError(t, err)
	assert.Equal(t, wantDigest, result.Digest)
	assert.Equal(t, "anonymous", result.AuthMethod)
	assert.Equal(t, "registry.example.com:5443", result.AuthRegistry)
}

func TestContainerRegistryService_GetImageDigest_HonorsCallerCancellation(t *testing.T) {
	started := make(chan struct{}, 1)
	svc := NewContainerRegistryService(nil, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, _ string, _ client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				select {
				case started <- struct{}{}:
				default:
				}
				<-ctx.Done()
				return client.DistributionInspectResult{}, ctx.Err()
			},
		}, nil
	}, nil, nil)
	t.Cleanup(svc.cache.StopJanitor)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := svc.ImageDigest(ctx, "registry.example.com/team/app:latest")
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "registry lookup did not start")
	}
	cancel()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		require.FailNow(t, "registry lookup did not honor caller cancellation")
	}
}

func TestContainerRegistryService_InspectImageDigest_UsesStoredDockerHubCredentialsOnFirstAttempt(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "https://index.docker.io/v1/", "docker-user", "docker-token")
	wantDigest := digest.FromString("stored-credentials").String()

	var calls int
	svc := NewContainerRegistryService(db, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				calls++
				assert.Equal(t, "docker.io/library/nginx:latest", imageRef)

				authCfg := decodeRegistryAuthInternal(t, options.EncodedRegistryAuth)
				assert.Equal(t, "docker-user", authCfg.Username)
				assert.Equal(t, "docker-token", authCfg.Password)
				assert.Equal(t, "https://index.docker.io/v1/", authCfg.ServerAddress)

				return client.DistributionInspectResult{
					Descriptor: ocispec.Descriptor{
						Digest: digest.Digest(wantDigest),
					},
				}, nil
			},
		}, nil
	}, nil, nil)

	result, err := svc.InspectImageDigest(t.Context(), "registry-1.docker.io/library/nginx:latest", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, wantDigest, result.Digest)
	assert.Equal(t, "credential", result.AuthMethod)
	assert.Equal(t, "docker-user", result.AuthUsername)
	assert.True(t, result.UsedCredential)
}

func TestContainerRegistryService_InspectImageDigest_UsesStoredDockerHubCredentialsForRegistryImage(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "https://index.docker.io/v1/", "docker-user", "docker-token")
	wantDigest := digest.FromString("stored-credentials-rate-limit").String()

	var calls int
	svc := NewContainerRegistryService(db, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				calls++
				assert.Equal(t, "docker.io/library/registry:3", imageRef)

				authCfg := decodeRegistryAuthInternal(t, options.EncodedRegistryAuth)
				assert.Equal(t, "docker-user", authCfg.Username)
				assert.Equal(t, "docker-token", authCfg.Password)
				assert.Equal(t, "https://index.docker.io/v1/", authCfg.ServerAddress)

				return client.DistributionInspectResult{
					Descriptor: ocispec.Descriptor{
						Digest: digest.Digest(wantDigest),
					},
				}, nil
			},
		}, nil
	}, nil, nil)

	result, err := svc.InspectImageDigest(t.Context(), "docker.io/library/registry:3", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, wantDigest, result.Digest)
	assert.Equal(t, "credential", result.AuthMethod)
	assert.Equal(t, "docker-user", result.AuthUsername)
	assert.True(t, result.UsedCredential)
}
